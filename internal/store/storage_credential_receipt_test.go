package store

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func Test存储轮换原回执重放与事务边界(t *testing.T) {
	database, _ := openTestStore(t)
	ctx := context.Background()
	if err := database.EnsureAuthSubject(ctx, AuthSubject{SubjectID: "storage-owner", ExternalSubject: "synthetic-storage-owner", DisplayName: "合成所有者", AccountStatus: "ACTIVE", CreatedAt: testTime, UpdatedAt: testTime}); err != nil {
		t.Fatal(err)
	}
	secret := func(id string, revision int64) EncryptedStorageSecret {
		return EncryptedStorageSecret{CredentialID: id, Revision: revision, KeyID: "synthetic-key", Nonce: []byte{1}, Ciphertext: []byte{2}}
	}
	if _, err := database.CreateStorageCredential(ctx, StorageCredentialCreate{StorageCredentialID: "storage-receipt", OwnerSubjectID: "storage-owner", DisplayName: "合成存储", Provider: "OSS", AccessKey: secret("synthetic-access-1", 1), SecretKey: secret("synthetic-secret-1", 1), RequestID: "create-storage", IdempotencyKey: "synthetic-create-key", RequestDigest: testFingerprint, CreatedAt: testTime}); err != nil {
		t.Fatal(err)
	}
	input := StorageCredentialRotate{StorageCredentialID: "storage-receipt", ActorSubjectID: "storage-owner", ExpectedRevision: 1, AccessKey: secret("synthetic-access-2", 2), SecretKey: secret("synthetic-secret-2", 2), RequestID: "rotate-storage-2", IdempotencyKey: "synthetic-rotate-key-2", RequestDigest: testFingerprint, UpdatedAt: testTime.Add(time.Second)}
	first, err := database.RotateStorageCredential(ctx, input)
	if err != nil || first.CurrentRevision != 2 {
		t.Fatal(first, err)
	}
	retry := input
	retry.AccessKey, retry.SecretKey = secret("different-random-envelope", 3), secret("different-random-secret", 3)
	retry.UpdatedAt = testTime.Add(time.Minute)
	for _, legacy := range []bool{false, true} {
		if legacy {
			if _, err := database.db.Exec(`UPDATE request_idempotency SET response_json='{}' WHERE operation='ROTATE_STORAGE_CREDENTIAL'`); err != nil {
				t.Fatal(err)
			}
		}
		result, err := database.RotateStorageCredential(ctx, retry)
		if err != nil || result.CurrentRevision != 2 || !result.UpdatedAt.Equal(first.UpdatedAt) {
			t.Fatal("原回执没有恢复", legacy, result, err)
		}
	}
	assertCount(t, database.db, `SELECT COUNT(*) FROM storage_credential_revisions`, 4)
	assertCount(t, database.db, `SELECT COUNT(*) FROM audit_events WHERE action='STORAGE_CREDENTIAL_ROTATED'`, 1)
	second := input
	second.ExpectedRevision, second.AccessKey, second.SecretKey = 2, secret("synthetic-access-3", 3), secret("synthetic-secret-3", 3)
	second.RequestID, second.IdempotencyKey, second.UpdatedAt = "rotate-storage-3", "synthetic-rotate-key-3", testTime.Add(2*time.Second)
	if _, err := database.RotateStorageCredential(ctx, second); err != nil {
		t.Fatal(err)
	}
	old, err := database.RotateStorageCredential(ctx, retry)
	if err != nil || old.CurrentRevision != 2 {
		t.Fatal("旧回执伪装成最新修订", old, err)
	}
	for _, mutate := range []func(*StorageCredentialRotate){func(v *StorageCredentialRotate) { v.RequestDigest = strings.Repeat("b", 64) }, func(v *StorageCredentialRotate) { v.ExpectedRevision = 2 }} {
		wrong := input
		mutate(&wrong)
		if _, err := database.RotateStorageCredential(ctx, wrong); !errors.Is(err, ErrIdempotencyConflict) {
			t.Fatal("异内容或版本没有冲突", err)
		}
	}
	if _, err := database.db.Exec(`UPDATE auth_subjects SET account_status='DISABLED' WHERE subject_id='storage-owner'`); err != nil {
		t.Fatal(err)
	}
	if _, err := database.RotateStorageCredential(ctx, input); !errors.Is(err, ErrStorageCredentialForbidden) {
		t.Fatal("停用主体仍可重放", err)
	}
	if _, err := database.db.Exec(`UPDATE auth_subjects SET account_status='ACTIVE' WHERE subject_id='storage-owner'`); err != nil {
		t.Fatal(err)
	}
	// 审计写入失败必须同时回滚两个新信封、主版本和回执，随后原请求仍可成功。
	third := second
	third.ExpectedRevision, third.AccessKey, third.SecretKey = 3, secret("synthetic-access-4", 4), secret("synthetic-secret-4", 4)
	third.RequestID, third.IdempotencyKey, third.UpdatedAt = "rotate-storage-4", "synthetic-rotate-key-4", testTime.Add(3*time.Second)
	if _, err := database.db.Exec(`CREATE TRIGGER synthetic_reject_rotation_audit BEFORE INSERT ON audit_events WHEN NEW.action='STORAGE_CREDENTIAL_ROTATED' BEGIN SELECT RAISE(ABORT,'synthetic audit rejection'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err := database.RotateStorageCredential(ctx, third); err == nil {
		t.Fatal("审计失败仍提交轮换")
	}
	assertCount(t, database.db, `SELECT COUNT(*) FROM storage_credential_revisions`, 6)
	assertCount(t, database.db, `SELECT COUNT(*) FROM request_idempotency WHERE operation='ROTATE_STORAGE_CREDENTIAL'`, 2)
	if _, err := database.db.Exec(`DROP TRIGGER synthetic_reject_rotation_audit`); err != nil {
		t.Fatal(err)
	}
	if _, err := database.RotateStorageCredential(ctx, third); err != nil {
		t.Fatal(err)
	}
	if err := database.DeleteStorageCredential(ctx, StorageCredentialDeletion{StorageCredentialID: input.StorageCredentialID, ActorSubjectID: input.ActorSubjectID, ExpectedRevision: 4, RequestID: "delete-storage", DeletedAt: testTime.Add(time.Minute)}); err != nil {
		t.Fatal(err)
	}
	if _, err := database.RotateStorageCredential(ctx, input); !errors.Is(err, ErrStorageCredentialNotFound) {
		t.Fatal("已删除对象仍可恢复", err)
	}
}
