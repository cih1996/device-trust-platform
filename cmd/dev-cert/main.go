package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"flag"
	"fmt"
	"log"
	"math/big"
	"os"
	"path/filepath"
	"time"
)

func main() {
	dir := flag.String("out", "work/certs", "private development TLS output")
	flag.Parse()
	if err := os.MkdirAll(*dir, 0700); err != nil {
		log.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(*dir, "localhost-cert.pem")); err == nil {
		log.Fatal("development certificate already exists; choose a fresh output directory")
	}
	now := time.Now().UTC()
	caKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	serverKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	serial, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 120))
	ca := &x509.Certificate{SerialNumber: serial, Subject: pkix.Name{CommonName: "Device Trust LOCAL TEST CA"}, NotBefore: now.Add(-time.Minute), NotAfter: now.Add(365 * 24 * time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign}
	rootDER, err := x509.CreateCertificate(rand.Reader, ca, ca, &caKey.PublicKey, caKey)
	if err != nil {
		log.Fatal(err)
	}
	leafSerial, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 120))
	leaf := &x509.Certificate{SerialNumber: leafSerial, Subject: pkix.Name{CommonName: "localhost"}, DNSNames: []string{"localhost"}, NotBefore: now.Add(-time.Minute), NotAfter: now.Add(90 * 24 * time.Hour), KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	leafDER, err := x509.CreateCertificate(rand.Reader, leaf, ca, &serverKey.PublicKey, caKey)
	if err != nil {
		log.Fatal(err)
	}
	private, err := x509.MarshalPKCS8PrivateKey(serverKey)
	if err != nil {
		log.Fatal(err)
	}
	files := map[string][]byte{"ca.pem": pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: rootDER}), "localhost-cert.pem": append(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: leafDER}), pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: rootDER})...), "localhost-key.pem": pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: private})}
	public, _ := x509.MarshalPKIXPublicKey(&serverKey.PublicKey)
	pin := sha256.Sum256(public)
	files["spki-pin.txt"] = []byte(base64.StdEncoding.EncodeToString(pin[:]))
	for name, data := range files {
		if err = os.WriteFile(filepath.Join(*dir, name), data, 0600); err != nil {
			log.Fatal(err)
		}
	}
	fmt.Println("Fresh localhost-only development TLS material generated; CA signing key was not saved.")
}
