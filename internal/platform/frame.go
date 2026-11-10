package platform

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/ecdsa"
	"crypto/hkdf"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
)

const frameHeaderSize = 142
const frameLimit = 256 << 10
const frameKeyInfo = "device-trust/frame/v1"

type secureFrame struct {
	Kind                                                    byte
	ChallengeID, KeyID                                      string
	PublicKey, Nonce, Header, Ciphertext, Signature, Signed []byte
}

func parseFrame(data []byte) (secureFrame, error) {
	var f secureFrame
	if len(data) < frameHeaderSize+16+2 || len(data) > frameLimit {
		return f, errors.New("invalid frame length")
	}
	if !bytes.Equal(data[:4], []byte("DTP4")) || (data[4] != 1 && data[4] != 2) {
		return f, errors.New("unsupported binary frame")
	}
	length := int(binary.BigEndian.Uint32(data[138:142]))
	end := frameHeaderSize + length
	if length < 16 || end+2 > len(data) {
		return f, errors.New("invalid encrypted payload length")
	}
	sigLength := int(binary.BigEndian.Uint16(data[end : end+2]))
	if sigLength < 8 || sigLength > 80 || end+2+sigLength != len(data) {
		return f, errors.New("invalid ECDSA signature length")
	}
	f = secureFrame{Kind: data[4], ChallengeID: hex.EncodeToString(data[5:29]), KeyID: hex.EncodeToString(data[29:61]), PublicKey: data[61:126], Nonce: data[126:138], Header: data[:142], Ciphertext: data[142:end], Signature: data[end+2:], Signed: data[:end]}
	return f, nil
}
func (f secureFrame) verify(key *ecdsa.PublicKey) error {
	hash := sha256.Sum256(f.Signed)
	if !ecdsa.VerifyASN1(key, hash[:], f.Signature) {
		return errors.New("invalid report signature")
	}
	return nil
}
func (f secureFrame) decrypt(privateKey, challengeNonce []byte) ([]byte, error) {
	private, err := ecdh.P256().NewPrivateKey(privateKey)
	if err != nil {
		return nil, err
	}
	public, err := ecdh.P256().NewPublicKey(f.PublicKey)
	if err != nil {
		return nil, err
	}
	secret, err := private.ECDH(public)
	if err != nil {
		return nil, err
	}
	key, err := hkdf.Key(sha256.New, secret, challengeNonce, frameKeyInfo, 32)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return gcm.Open(nil, f.Nonce, f.Ciphertext, f.Header)
}
