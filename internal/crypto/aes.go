package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"io"
)

// ErrInvalidKeySize is returned when the AES key size is not 32 bytes (256 bits).
var ErrInvalidKeySize = errors.New("crypto: invalid AES key size, expected 32 bytes")

// ErrCiphertextTooShort is returned when ciphertext is too short to contain a nonce.
var ErrCiphertextTooShort = errors.New("crypto: ciphertext too short")

// ErrDecryptionFailed is returned when decryption or authentication fails.
var ErrDecryptionFailed = errors.New("crypto: decryption failed")

// Encryptor defines the interface for authenticated encryption.
type Encryptor interface {
	Encrypt(plaintext []byte, aad []byte) ([]byte, error)
	Decrypt(ciphertext []byte, aad []byte) ([]byte, error)
}

type aesGCMEncryptor struct {
	aead cipher.AEAD
}

// NewAESGCMEncryptor creates a new Encryptor using AES-256-GCM.
func NewAESGCMEncryptor(key []byte) (Encryptor, error) {
	if len(key) != 32 {
		return nil, ErrInvalidKeySize
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	return &aesGCMEncryptor{
		aead: aead,
	}, nil
}

// Encrypt encrypts the plaintext using AES-GCM with the provided AAD.
func (e *aesGCMEncryptor) Encrypt(plaintext []byte, aad []byte) ([]byte, error) {
	nonce := make([]byte, e.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}

	// Seal appends the ciphertext and authentication tag to dst.
	// We can prepend the nonce to the ciphertext by passing nonce as dst.
	ciphertext := e.aead.Seal(nonce, nonce, plaintext, aad)
	return ciphertext, nil
}

// Decrypt decrypts the ciphertext using AES-GCM with the provided AAD.
func (e *aesGCMEncryptor) Decrypt(ciphertext []byte, aad []byte) ([]byte, error) {
	nonceSize := e.aead.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, ErrCiphertextTooShort
	}

	nonce := ciphertext[:nonceSize]
	actualCiphertext := ciphertext[nonceSize:]

	plaintext, err := e.aead.Open(nil, nonce, actualCiphertext, aad)
	if err != nil {
		return nil, ErrDecryptionFailed
	}

	return plaintext, nil
}

// Zeroize clears the contents of a byte slice to prevent sensitive data remanence.
func Zeroize(b []byte) {
	for i := range b {
		b[i] = 0
	}
}
