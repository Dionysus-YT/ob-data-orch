package controlplane

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"ob-data-orch/internal/buildinfo"
	"ob-data-orch/internal/credential"
	"ob-data-orch/internal/store"
)

// randomReceiptEncryptor 只为隔离测试制造每次不同的信封，确认回执摘要不依赖加密随机数。
type randomReceiptEncryptor struct{}

func (randomReceiptEncryptor) Encrypt(key string, ref credential.Reference, _ []byte) (credential.Envelope, error) {
	nonce := make([]byte, 12)
	if _, err := rand.Read(nonce); err != nil {
		return credential.Envelope{}, err
	}
	return credential.Envelope{FormatVersion: credential.FormatVersion, KeyID: key, Reference: ref, Nonce: nonce, Ciphertext: []byte("synthetic-encrypted-value")}, nil
}

func Test存储轮换HTTP原请求恢复旧修订(t *testing.T) {
	database, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "receipt.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	now := time.Now().UTC()
	if err := database.EnsureAuthSubject(context.Background(), store.AuthSubject{SubjectID: "synthetic-subject", ExternalSubject: "synthetic-receipt-subject", DisplayName: "合成用户", AccountStatus: "ACTIVE", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	handler := NewHandlerWithDependencies(buildinfo.Info{Version: "test"}, Dependencies{Identity: browserOnlyIdentityProvider{}, CSRF: allowedCSRF{}, StorageCredentials: database, Encryptor: randomReceiptEncryptor{}, CredentialKeyID: "synthetic-key"})
	body := `{"displayName":"合成凭据","provider":"OSS","accessKey":"synthetic-access","secretKey":"synthetic-secret"}`
	request := func(path, key, revision string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(body))
		r.Header.Set("Idempotency-Key", key)
		if revision != "" {
			r.Header.Set("If-Match", revision)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	created := request("/api/v1/storage-credentials", "synthetic-create-receipt", "")
	var result struct {
		Item struct {
			ID string `json:"id"`
		} `json:"item"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &result); err != nil || created.Code != 201 || result.Item.ID == "" {
		t.Fatal("创建失败", created.Code, err)
	}
	path := "/api/v1/storage-credentials/" + result.Item.ID + ":rotate"
	first := request(path, "synthetic-rotate-receipt", `"rev-1"`)
	if first.Code != 200 || !bytes.Contains(first.Body.Bytes(), []byte(`"currentRevision":2`)) {
		t.Fatal("轮换失败", first.Code)
	}
	second := request(path, "synthetic-rotate-next", `"rev-2"`)
	if second.Code != 200 || !bytes.Contains(second.Body.Bytes(), []byte(`"currentRevision":3`)) {
		t.Fatal("后续轮换失败", second.Code)
	}
	old := request(path, "synthetic-rotate-receipt", `"rev-1"`)
	if old.Code != 200 || !bytes.Contains(old.Body.Bytes(), []byte(`"currentRevision":2`)) || bytes.Contains(old.Body.Bytes(), []byte("synthetic-secret")) {
		t.Fatal("原 If-Match 未恢复原安全回执", old.Code)
	}
	reference, err := database.GetStorageCredentialReference(context.Background(), result.Item.ID)
	if err != nil || reference.Revision != 3 {
		t.Fatal("重放改写了最新版本", reference.Revision, err)
	}
	conflict := request(path, "synthetic-rotate-receipt", `"rev-2"`)
	if conflict.Code != 409 {
		t.Fatal("同键异版本没有冲突", conflict.Code)
	}
}
