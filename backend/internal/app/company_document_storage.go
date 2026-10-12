package app

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const companyDocumentMaxBytes = 20 << 20
const companyCompressedKeyPrefix = "kyc-gz1."

// This marker lives INSIDE the authenticated ciphertext. Its first NUL byte
// cannot be confused with any accepted PDF, raster image, DOCX or XLSX signature.
var companyCompressedMagic = []byte("\x00EURO-KYC-GZIP\x01")
var errCompanyCompressionNoSaving = errors.New("compression does not save enough space")

type companyCompressionWriter struct {
	buffer bytes.Buffer
	limit  int
}

func (w *companyCompressionWriter) Write(p []byte) (int, error) {
	if len(p) > w.limit-w.buffer.Len() {
		return 0, errCompanyCompressionNoSaving
	}
	return w.buffer.Write(p)
}

// Only new uploads use this envelope. Already-compressed or small files keep
// their original storage representation; no existing file is rewritten.
func prepareCompanyDocumentStorage(ctx context.Context, plain []byte) ([]byte, bool, error) {
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	if len(plain) == 0 || len(plain) > companyDocumentMaxBytes {
		return nil, false, bad("File must be between 1 byte and 20 MB.")
	}
	minSaving := max(128, len(plain)/100)
	headerSize := len(companyCompressedMagic) + 4
	if len(plain) <= minSaving+headerSize {
		return plain, false, nil
	}
	compressionCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	output := &companyCompressionWriter{limit: len(plain) - minSaving - headerSize}
	writer := gzip.NewWriter(output)
	_, err := io.CopyBuffer(writer, companyPreviewContextReader{ctx: compressionCtx, reader: bytes.NewReader(plain)}, make([]byte, 64<<10))
	closeErr := writer.Close()
	if err == nil {
		err = closeErr
	}
	if ctx.Err() != nil {
		return nil, false, ctx.Err()
	}
	// A bounded compression attempt may safely fall back to original bytes.
	if errors.Is(err, errCompanyCompressionNoSaving) || errors.Is(err, context.DeadlineExceeded) {
		return plain, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	result := make([]byte, headerSize, headerSize+output.buffer.Len())
	copy(result, companyCompressedMagic)
	binary.BigEndian.PutUint32(result[len(companyCompressedMagic):], uint32(len(plain)))
	result = append(result, output.buffer.Bytes()...)
	return result, true, nil
}

// Decompression happens only after AES authentication. A single gzip member is
// consumed through EOF for CRC verification; trailing members/data are rejected.
func restoreCompanyDocumentStorage(ctx context.Context, stored []byte, originalSize int64, originalSHA string, compressedKey bool) ([]byte, error) {
	invalid := bad("Document is damaged or exceeds the permitted size.")
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(stored) == 0 || len(stored) > companyDocumentMaxBytes {
		return nil, invalid
	}
	marked := bytes.HasPrefix(stored, companyCompressedMagic)
	if !marked {
		if compressedKey {
			return nil, invalid
		}
		return stored, nil // Existing uncompressed encrypted files remain unchanged.
	}
	headerSize := len(companyCompressedMagic) + 4
	if len(stored) <= headerSize {
		return nil, invalid
	}
	declared := int64(binary.BigEndian.Uint32(stored[len(companyCompressedMagic):headerSize]))
	if declared <= 0 || declared > companyDocumentMaxBytes || declared != originalSize {
		return nil, invalid
	}
	expectedHash, err := hex.DecodeString(originalSHA)
	if err != nil || len(expectedHash) != sha256.Size {
		return nil, invalid
	}
	input := bytes.NewReader(stored[headerSize:]) // implements io.ByteReader
	reader, err := gzip.NewReader(input)
	if err != nil {
		return nil, invalid
	}
	defer reader.Close()
	reader.Multistream(false)
	decodeCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	plain, err := io.ReadAll(io.LimitReader(companyPreviewContextReader{ctx: decodeCtx, reader: reader}, declared+1))
	if err != nil || int64(len(plain)) != declared || input.Len() != 0 {
		return nil, invalid
	}
	sum := sha256.Sum256(plain)
	if !bytes.Equal(sum[:], expectedHash) {
		return nil, invalid
	}
	return plain, nil
}

func companyStorageMetadata(originalSize, storedSize int64, compressed bool) M {
	// Existing AES-GCM format: 12-byte nonce and 16-byte authentication tag.
	saved := max(int64(0), originalSize+28-storedSize)
	method := "none"
	if compressed {
		method = "gzip"
	} else {
		saved = 0
	}
	return M{"original_size": originalSize, "stored_size": storedSize, "saved_bytes": saved, "compression": method}
}

// Private file lists expose sizes, never filesystem storage keys. Missing
// legacy files keep their existing metadata and do not block the company form.
func (a *App) addCompanyStorageMetadata(doc M) {
	key := str(doc, "storage_key")
	delete(doc, "storage_key")
	if key == "" || filepath.Base(key) != key {
		return
	}
	info, err := os.Stat(filepath.Join(a.FileRoot, key))
	if err != nil || !info.Mode().IsRegular() {
		return
	}
	for field, value := range companyStorageMetadata(id(doc, "size"), info.Size(), strings.HasPrefix(key, companyCompressedKeyPrefix)) {
		doc[field] = value
	}
}

func readCompanyEncryptedFile(path string) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	encrypted, err := io.ReadAll(io.LimitReader(file, companyDocumentMaxBytes+29))
	if err != nil {
		return nil, err
	}
	if len(encrypted) > companyDocumentMaxBytes+28 {
		return nil, bad("Document is damaged or exceeds the permitted size.")
	}
	return encrypted, nil
}
