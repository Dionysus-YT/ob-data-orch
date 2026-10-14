package credential

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"testing"
)

func TestSysSecurityMaterialUsesOfficialPropertyAndRejectsLineInjection(t *testing.T) {
	material, err := GenerateSecurityMaterialWithSys([]byte("synthetic_db_password"), []byte("synthetic_sys_password"))
	if err != nil {
		t.Fatal(err)
	}
	defer material.Destroy()
	block, _ := pem.Decode(material.privateKeyPEM)
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	plaintext, err := rsa.DecryptPKCS1v15(rand.Reader, key.(*rsa.PrivateKey), material.ciphertext)
	if err != nil {
		t.Fatal(err)
	}
	defer Zero(plaintext)
	if !bytes.Equal(plaintext, []byte("oceanbase.jdbc.password=synthetic_db_password\noceanbase.jdbc.sys.password=synthetic_sys_password\n")) {
		t.Fatal("官方加密属性不匹配")
	}
	if _, err := GenerateSecurityMaterialWithSys([]byte("synthetic_db_password"), []byte("synthetic\nother=value")); err == nil {
		t.Fatal("sys 配置行注入被接受")
	}
}
