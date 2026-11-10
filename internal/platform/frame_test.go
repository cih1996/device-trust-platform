package platform

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/x509"
	"encoding/json"
	"os"
	"testing"
)

func TestObfuscatedJNINativeFrameInteroperability(t *testing.T) {
	data, err := os.ReadFile("testdata/native-frame.json")
	if err != nil {
		t.Fatal(err)
	}
	var vector struct {
		Frame            []byte `json:"frame"`
		ServerPrivateKey []byte `json:"serverPrivateKey"`
		Nonce            []byte `json:"nonce"`
		PublicKey        []byte `json:"publicKey"`
		Payload          []byte `json:"payload"`
	}
	if err = json.Unmarshal(data, &vector); err != nil {
		t.Fatal(err)
	}
	frame, err := parseFrame(vector.Frame)
	if err != nil {
		t.Fatal(err)
	}
	public, err := x509.ParsePKIXPublicKey(vector.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	key := public.(*ecdsa.PublicKey)
	if err = frame.verify(key); err != nil {
		t.Fatal(err)
	}
	payload, err := frame.decrypt(vector.ServerPrivateKey, vector.Nonce)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(payload, vector.Payload) {
		t.Fatalf("payload mismatch: %s", payload)
	}
	for _, index := range []int{4, 5, 29, 61, 126, 138, 142, len(vector.Frame) - 1} {
		tampered := append([]byte{}, vector.Frame...)
		tampered[index] ^= 1
		changed, err := parseFrame(tampered)
		if err != nil {
			continue
		}
		if err = changed.verify(key); err == nil {
			t.Fatalf("tampered byte %d retained valid signature", index)
		}
	}
	wrongNonce := append([]byte{}, vector.Nonce...)
	wrongNonce[0] ^= 1
	if _, err = frame.decrypt(vector.ServerPrivateKey, wrongNonce); err == nil {
		t.Fatal("wrong challenge decrypted frame")
	}
}
