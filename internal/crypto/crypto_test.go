package crypto

import (
	"bytes"
	"testing"
)

func TestAESGCM_EncryptDecrypt_Roundtrip(t *testing.T) {
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}

	enc, err := NewAESGCMEncryptor(key)
	if err != nil {
		t.Fatalf("Failed to create encryptor: %v", err)
	}

	plaintext := []byte("secret PAN data")
	aad := []byte("tenant-123")

	ciphertext, err := enc.Encrypt(plaintext, aad)
	if err != nil {
		t.Fatalf("Encryption failed: %v", err)
	}

	decrypted, err := enc.Decrypt(ciphertext, aad)
	if err != nil {
		t.Fatalf("Decryption failed: %v", err)
	}

	if !bytes.Equal(plaintext, decrypted) {
		t.Errorf("Decrypted data %q does not match original plaintext %q", decrypted, plaintext)
	}
}

func TestAESGCM_DifferentNonce(t *testing.T) {
	key := make([]byte, 32)
	enc, _ := NewAESGCMEncryptor(key)

	plaintext := []byte("data")
	aad := []byte("aad")

	ct1, _ := enc.Encrypt(plaintext, aad)
	ct2, _ := enc.Encrypt(plaintext, aad)

	if bytes.Equal(ct1, ct2) {
		t.Error("Two encryptions of the same plaintext produced identical ciphertexts")
	}
}

func TestAESGCM_WrongKey(t *testing.T) {
	key1 := make([]byte, 32)
	key1[0] = 1
	enc1, _ := NewAESGCMEncryptor(key1)

	key2 := make([]byte, 32)
	key2[0] = 2
	enc2, _ := NewAESGCMEncryptor(key2)

	plaintext := []byte("data")
	aad := []byte("aad")

	ciphertext, _ := enc1.Encrypt(plaintext, aad)

	_, err := enc2.Decrypt(ciphertext, aad)
	if err == nil {
		t.Error("Decryption with wrong key should have failed")
	}
}

func TestAESGCM_WrongAAD(t *testing.T) {
	key := make([]byte, 32)
	enc, _ := NewAESGCMEncryptor(key)

	plaintext := []byte("data")
	aad1 := []byte("tenant-1")
	aad2 := []byte("tenant-2")

	ciphertext, _ := enc.Encrypt(plaintext, aad1)

	_, err := enc.Decrypt(ciphertext, aad2)
	if err == nil {
		t.Error("Decryption with wrong AAD should have failed")
	}
}

func TestAESGCM_TamperedCiphertext(t *testing.T) {
	key := make([]byte, 32)
	enc, _ := NewAESGCMEncryptor(key)

	plaintext := []byte("data")
	aad := []byte("aad")

	ciphertext, _ := enc.Encrypt(plaintext, aad)

	// Tamper with ciphertext
	ciphertext[len(ciphertext)-1] ^= 1

	_, err := enc.Decrypt(ciphertext, aad)
	if err == nil {
		t.Error("Decryption of tampered ciphertext should have failed")
	}
}

func TestAESGCM_InvalidKeyLength(t *testing.T) {
	_, err := NewAESGCMEncryptor(make([]byte, 31))
	if err != ErrInvalidKeySize {
		t.Errorf("Expected ErrInvalidKeySize, got %v", err)
	}

	_, err = NewAESGCMEncryptor(make([]byte, 33))
	if err != ErrInvalidKeySize {
		t.Errorf("Expected ErrInvalidKeySize, got %v", err)
	}
}

func TestBlindIndex_Deterministic(t *testing.T) {
	key := make([]byte, 32)
	idx, _ := NewHMACBlindIndexer(key)

	input := []byte("data")
	out1 := idx.ComputeIndex(input)
	out2 := idx.ComputeIndex(input)

	if out1 != out2 {
		t.Error("Blind index is not deterministic")
	}
}

func TestBlindIndex_DifferentInputs(t *testing.T) {
	key := make([]byte, 32)
	idx, _ := NewHMACBlindIndexer(key)

	out1 := idx.ComputeIndex([]byte("data1"))
	out2 := idx.ComputeIndex([]byte("data2"))

	if out1 == out2 {
		t.Error("Different inputs produced same blind index")
	}
}

func TestBlindIndex_DifferentKeys(t *testing.T) {
	key1 := make([]byte, 32)
	key1[0] = 1
	idx1, _ := NewHMACBlindIndexer(key1)

	key2 := make([]byte, 32)
	key2[0] = 2
	idx2, _ := NewHMACBlindIndexer(key2)

	input := []byte("data")
	out1 := idx1.ComputeIndex(input)
	out2 := idx2.ComputeIndex(input)

	if out1 == out2 {
		t.Error("Different keys produced same blind index")
	}
}

func TestBlindIndex_InvalidKeyLength(t *testing.T) {
	_, err := NewHMACBlindIndexer(make([]byte, 31))
	if err != ErrInvalidHMACKeySize {
		t.Errorf("Expected ErrInvalidHMACKeySize, got %v", err)
	}
}

func TestZeroize(t *testing.T) {
	data := []byte{1, 2, 3, 4, 5}
	Zeroize(data)

	for _, b := range data {
		if b != 0 {
			t.Errorf("Zeroize failed: slice contains non-zero byte %v", b)
		}
	}
}
