package deployment

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"

	"ob-data-orch/internal/credential"
)

// TLSConfig 使用持久信任根和自动续期的服务证书，Agent 升级无需重新分发信任。
func TLSConfig(directory, publicURL string) (*tls.Config, error) {
	parsed, err := validatePublicURL(publicURL)
	if err != nil {
		return nil, err
	}
	keyPath := filepath.Join(directory, "server-trust-key.json")
	caPath := filepath.Join(directory, "control-plane-ca.pem")
	_, caErr := os.Stat(caPath)
	_, keyErr := os.Stat(keyPath)
	if (caErr == nil) != (keyErr == nil) {
		return nil, errors.New("服务信任材料不完整，请恢复原文件")
	}
	seed, err := credential.LoadOrCreateRootKey(keyPath, "server-trust-v1")
	if err != nil {
		return nil, err
	}
	defer credential.Zero(seed)
	key, err := ecdsa.ParseRawPrivateKey(elliptic.P256(), seed)
	if err != nil {
		return nil, errors.New("服务信任私钥无效")
	}
	var ca *x509.Certificate
	if caErr == nil {
		content, err := os.ReadFile(caPath)
		if err != nil {
			return nil, err
		}
		block, _ := pem.Decode(content)
		if block == nil {
			return nil, errors.New("服务信任证书无效")
		}
		ca, err = x509.ParseCertificate(block.Bytes)
		if err != nil {
			return nil, err
		}
		pub, ok := ca.PublicKey.(*ecdsa.PublicKey)
		if !ok || !pub.Equal(key.Public()) || !ca.IsCA {
			return nil, errors.New("服务信任根不匹配")
		}
	} else {
		now := time.Now()
		serial, err := randomSerial()
		if err != nil {
			return nil, err
		}
		template := &x509.Certificate{SerialNumber: serial, Subject: pkix.Name{CommonName: "OB Data Orch"}, NotBefore: now.Add(-time.Hour), NotAfter: now.AddDate(20, 0, 0), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature}
		der, err := x509.CreateCertificate(rand.Reader, template, template, key.Public(), key)
		if err != nil {
			return nil, err
		}
		ca, err = x509.ParseCertificate(der)
		if err != nil {
			return nil, err
		}
		if err := os.WriteFile(caPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0600); err != nil {
			return nil, err
		}
	}
	var mu sync.Mutex
	var certificate *tls.Certificate
	getCertificate := func(*tls.ClientHelloInfo) (*tls.Certificate, error) {
		mu.Lock()
		defer mu.Unlock()
		now := time.Now()
		if now.Before(ca.NotBefore) || !now.Add(24*time.Hour).Before(ca.NotAfter) {
			return nil, errors.New("服务信任根已过期")
		}
		if certificate != nil && now.Add(30*24*time.Hour).Before(certificate.Leaf.NotAfter) {
			return certificate, nil
		}
		private, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			return nil, err
		}
		serial, err := randomSerial()
		if err != nil {
			return nil, err
		}
		template := &x509.Certificate{SerialNumber: serial, Subject: pkix.Name{CommonName: parsed.Hostname()}, NotBefore: now.Add(-time.Hour), NotAfter: now.Add(90 * 24 * time.Hour), KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
		if ip := net.ParseIP(parsed.Hostname()); ip != nil {
			template.IPAddresses = []net.IP{ip}
		} else {
			template.DNSNames = []string{parsed.Hostname()}
		}
		der, err := x509.CreateCertificate(rand.Reader, template, ca, private.Public(), key)
		if err != nil {
			return nil, err
		}
		leaf, err := x509.ParseCertificate(der)
		if err != nil {
			return nil, err
		}
		certificate = &tls.Certificate{Certificate: [][]byte{der, ca.Raw}, PrivateKey: private, Leaf: leaf}
		return certificate, nil
	}
	if _, err := getCertificate(nil); err != nil {
		return nil, err
	}
	return &tls.Config{MinVersion: tls.VersionTLS12, GetCertificate: getCertificate}, nil
}

func randomSerial() (*big.Int, error) {
	return rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
}
