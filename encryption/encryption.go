package encryption

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base32"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	"image/png"
	_ "image/png"
	"io"
	"log"
	"os"

	_ "golang.org/x/image/bmp"
	_ "golang.org/x/image/tiff"
	_ "golang.org/x/image/webp"
)

// to generate a random 32-byte master key, use: 'go run generate.go' in the key folder

var MasterKey []byte

func InitEnv() {
	keyHex := os.Getenv("MASTER_KEY")
	if keyHex == "" {
		log.Fatal("MASTER_KEY environment variable is not set")
	}

	decoded, err := hex.DecodeString(keyHex)
	if err != nil {
		log.Fatalf("MASTER_KEY must be a valid hex string: %v", err)
	}

	if len(decoded) != 32 {
		log.Fatalf("MASTER_KEY must be 32 bytes; got %d", len(decoded))
	}

	MasterKey = decoded
}

// creates a 160-bit (20 byte) Base32 secret
func GenerateRandomSecret() (string, error) {
	secret := make([]byte, 20)
	if _, err := io.ReadFull(rand.Reader, secret); err != nil {
		return "", err
	}
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(secret), nil
}

// encrypt with AES-GCM, handles raw bytes (images, files, etc)
func EncryptData(unencryptedData []byte, masterKey []byte) ([]byte, error) {
	block, err := aes.NewCipher(masterKey)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}

	return gcm.Seal(nonce, nonce, unencryptedData, nil), nil
}

// returns the original raw bytes
func DecryptData(encryptedData []byte, masterKey []byte) ([]byte, error) {
	block, err := aes.NewCipher(masterKey)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonceSize := gcm.NonceSize()
	if len(encryptedData) < nonceSize {
		return nil, fmt.Errorf("ciphertext too short")
	}

	nonce, ciphertext := encryptedData[:nonceSize], encryptedData[nonceSize:]
	return gcm.Open(nil, nonce, ciphertext, nil)
}

func StringToBytes(s string) []byte {
	return []byte(s)
}

func BytesToString(b []byte) string {
	return string(b)
}

func Int64ToBytes(n int64) []byte {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, uint64(n))
	return b
}

func BytesToInt64(b []byte) int64 {
	return int64(binary.BigEndian.Uint64(b))
}

func ImageToBytes(img image.Image) ([]byte, error) {
	buf := new(bytes.Buffer)
	err := png.Encode(buf, img)
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func BytesToImage(b []byte) (image.Image, string, error) {
	img, format, err := image.Decode(bytes.NewReader(b))
	return img, format, err
}
