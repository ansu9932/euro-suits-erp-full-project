package app

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"math/rand/v2"
	"os"
	"path/filepath"
	"testing"
)

func companyDocumentHash(value []byte) string {
	sum := sha256.Sum256(value)
	return hex.EncodeToString(sum[:])
}
func TestCompanyDocumentStorageLosslessAndSelective(t *testing.T) {
	original := append([]byte("%PDF-1.7\n"), bytes.Repeat([]byte("Repeated document text.\n"), 10000)...)
	stored, compressed, err := prepareCompanyDocumentStorage(context.Background(), original)
	if err != nil || !compressed || len(stored) >= len(original)-128 {
		t.Fatalf("compression ineffective: %d -> %d; %v", len(original), len(stored), err)
	}
	again, _, err := prepareCompanyDocumentStorage(context.Background(), original)
	if err != nil || !bytes.Equal(stored, again) {
		t.Fatal("versioned compressed representation is nondeterministic")
	}
	plain, err := restoreCompanyDocumentStorage(context.Background(), stored, int64(len(original)), companyDocumentHash(original), true)
	if err != nil || !bytes.Equal(plain, original) {
		t.Fatal("compressed document did not roundtrip byte-exact", err)
	}
	// Incompressible input must not increase storage or change representation.
	random := rand.New(rand.NewPCG(7, 11))
	noise := make([]byte, 65536)
	for i := range noise {
		noise[i] = byte(random.Uint32())
	}
	for _, raw := range [][]byte{[]byte("%PDF-1.4\nsmall"), noise} {
		encoded, compressed, err := prepareCompanyDocumentStorage(context.Background(), raw)
		if err != nil || compressed || !bytes.Equal(encoded, raw) {
			t.Fatal("small/incompressible document changed", err)
		}
		got, err := restoreCompanyDocumentStorage(context.Background(), raw, int64(len(raw)), companyDocumentHash(raw), false)
		if err != nil || !bytes.Equal(got, raw) {
			t.Fatal("legacy uncompressed document changed", err)
		}
	}
	metadata := companyStorageMetadata(int64(len(original)), int64(len(stored)+28), true)
	if id(metadata, "saved_bytes") != int64(len(original)-len(stored)) || str(metadata, "compression") != "gzip" {
		t.Fatalf("inaccurate storage savings: %v", metadata)
	}
}

func TestCompanyDocumentStorageRejectsDamagedAndOversized(t *testing.T) {
	original := append([]byte("%PDF-1.7\n"), bytes.Repeat([]byte("Sensitive content\n"), 10000)...)
	stored, _, err := prepareCompanyDocumentStorage(context.Background(), original)
	if err != nil {
		t.Fatal(err)
	}
	hash := companyDocumentHash(original)
	headerSize := len(companyCompressedMagic) + 4
	for _, tc := range []struct {
		name   string
		modify func([]byte) []byte
		size   int64
		hash   string
	}{
		{"truncated marker", func(s []byte) []byte { return s[:len(companyCompressedMagic)+2] }, int64(len(original)), hash},
		{"unknown version", func(s []byte) []byte { s[len(companyCompressedMagic)-1]++; return s }, int64(len(original)), hash},
		{"truncated gzip", func(s []byte) []byte { return s[:len(s)-4] }, int64(len(original)), hash},
		{"crc failure", func(s []byte) []byte { s[len(s)-8] ^= 1; return s }, int64(len(original)), hash},
		{"trailing bytes", func(s []byte) []byte { return append(s, 1) }, int64(len(original)), hash},
		{"multiple gzip members", func(s []byte) []byte { return append(s, s[headerSize:]...) }, int64(len(original)), hash},
		{"declared tiny bomb", func(s []byte) []byte {
			binary.BigEndian.PutUint32(s[len(companyCompressedMagic):headerSize], 2)
			return s
		}, 2, hash},
		{"over limit", func(s []byte) []byte {
			binary.BigEndian.PutUint32(s[len(companyCompressedMagic):headerSize], companyDocumentMaxBytes+1)
			return s
		}, companyDocumentMaxBytes + 1, hash},
		{"wrong original size", func(s []byte) []byte { return s }, int64(len(original) - 1), hash},
		{"wrong SHA", func(s []byte) []byte { return s }, int64(len(original)), companyDocumentHash([]byte("wrong"))},
		{"invalid SHA", func(s []byte) []byte { return s }, int64(len(original)), "bad"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			value := tc.modify(bytes.Clone(stored))
			if _, err := restoreCompanyDocumentStorage(context.Background(), value, tc.size, tc.hash, true); err == nil {
				t.Fatal("accepted damaged compressed document")
			}
		})
	}
	if _, err := restoreCompanyDocumentStorage(context.Background(), original, int64(len(original)), hash, true); err == nil {
		t.Fatal("compressed storage key accepted an unmarked payload")
	}
	if _, _, err := prepareCompanyDocumentStorage(context.Background(), make([]byte, companyDocumentMaxBytes+1)); err == nil {
		t.Fatal("accepted oversized original")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := prepareCompanyDocumentStorage(ctx, original); err == nil {
		t.Fatal("compression ignored cancellation")
	}
	if _, err := restoreCompanyDocumentStorage(ctx, stored, int64(len(original)), hash, true); err == nil {
		t.Fatal("decompression ignored cancellation")
	}
}

func TestCompanyDocumentStorageMetadataNeverExposesKey(t *testing.T) {
	root := t.TempDir()
	key := companyCompressedKeyPrefix + "fixture"
	if err := os.WriteFile(filepath.Join(root, key), make([]byte, 100), 0600); err != nil {
		t.Fatal(err)
	}
	app := &App{FileRoot: root}
	doc := M{"storage_key": key, "size": int64(1000)}
	app.addCompanyStorageMetadata(doc)
	if _, exists := doc["storage_key"]; exists {
		t.Fatal("private storage key exposed")
	}
	if id(doc, "stored_size") != 100 || id(doc, "saved_bytes") != 928 || str(doc, "compression") != "gzip" {
		t.Fatal("incorrect disk metadata", doc)
	}
	for _, key := range []string{"missing", "../outside"} {
		doc := M{"storage_key": key, "size": int64(5)}
		app.addCompanyStorageMetadata(doc)
		if _, exists := doc["storage_key"]; exists {
			t.Fatal("unavailable storage key exposed")
		}
		if _, exists := doc["stored_size"]; exists {
			t.Fatal("invented missing-file storage metadata")
		}
	}
	oversized := filepath.Join(root, "oversized")
	file, err := os.Create(oversized)
	if err != nil {
		t.Fatal(err)
	}
	if err = file.Truncate(companyDocumentMaxBytes + 29); err != nil {
		t.Fatal(err)
	}
	file.Close()
	if _, err := readCompanyEncryptedFile(oversized); err == nil {
		t.Fatal("unbounded encrypted file accepted")
	}
}

func TestCompanyDocumentStorageMaximumSizeAndExpansionBound(t *testing.T) {
	original := bytes.Repeat([]byte("A"), companyDocumentMaxBytes)
	copy(original, []byte("%PDF-1.7\n"))
	stored, compressed, err := prepareCompanyDocumentStorage(context.Background(), original)
	if err != nil || !compressed {
		t.Fatal("maximum-size original compression failed", err)
	}
	restored, err := restoreCompanyDocumentStorage(context.Background(), stored, int64(len(original)), companyDocumentHash(original), true)
	if err != nil || !bytes.Equal(restored, original) {
		t.Fatal("maximum-size original did not roundtrip", err)
	}
	var payload bytes.Buffer
	writer := gzip.NewWriter(&payload)
	if _, err := writer.Write(original); err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Write([]byte("X")); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	malicious := append(bytes.Clone(stored[:len(companyCompressedMagic)+4]), payload.Bytes()...)
	if _, err := restoreCompanyDocumentStorage(context.Background(), malicious, int64(len(original)), companyDocumentHash(original), true); err == nil {
		t.Fatal("accepted expansion beyond 20 MB")
	}
}

func TestCompanyDocumentLegacyRandomKeyPrefix(t *testing.T) {
	// Base64url random tokens can begin gz1- but never contain a period.
	root := t.TempDir()
	key := "kyc-gz1-legacy-random-token"
	if err := os.WriteFile(filepath.Join(root, key), make([]byte, 128), 0600); err != nil {
		t.Fatal(err)
	}
	doc := M{"storage_key": key, "size": int64(100)}
	(&App{FileRoot: root}).addCompanyStorageMetadata(doc)
	if str(doc, "compression") != "none" || id(doc, "saved_bytes") != 0 {
		t.Fatal("legacy random key misidentified as compressed")
	}
	original := []byte("%PDF-1.4\nLegacy")
	restored, err := restoreCompanyDocumentStorage(context.Background(), original, int64(len(original)), companyDocumentHash(original), false)
	if err != nil || !bytes.Equal(restored, original) {
		t.Fatal("legacy random key blocks original download", err)
	}
}
