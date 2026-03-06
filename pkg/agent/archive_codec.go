package agent

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"path"
	"strings"
	"time"

	"github.com/tinoosan/agen8/pkg/types"
)

type archiveSourceFile struct {
	Name  string
	Data  []byte
	Mtime *time.Time
}

type archiveDecodedEntry struct {
	Name      string
	IsDir     bool
	SizeBytes int64
	Mtime     *int64
	Data      []byte
}

type archiveListResult struct {
	Entries        []types.ArchiveEntry
	TotalSizeBytes int64
	Truncated      bool
}

type archiveCodec interface {
	Write(files []archiveSourceFile, includeMetadata bool) (data []byte, totalSizeBytes int64, err error)
	List(data []byte, limit int) (archiveListResult, error)
	Decode(data []byte) ([]archiveDecodedEntry, error)
}

func archiveCodecForCreate(format string) (archiveCodec, string, error) {
	switch normalizeArchiveFormat(format) {
	case "zip":
		return zipArchiveCodec{}, "zip", nil
	case "tar":
		return tarArchiveCodec{gzip: false}, "tar", nil
	case "tar.gz":
		return tarArchiveCodec{gzip: true}, "tar.gz", nil
	default:
		return nil, "", fmt.Errorf("unsupported archive format %q", format)
	}
}

func archiveCodecForPath(sourcePath string) (archiveCodec, string, error) {
	p := strings.ToLower(strings.TrimSpace(sourcePath))
	switch {
	case strings.HasSuffix(p, ".zip"):
		return zipArchiveCodec{}, "zip", nil
	case strings.HasSuffix(p, ".tar.gz"), strings.HasSuffix(p, ".tgz"):
		return tarArchiveCodec{gzip: true}, "tar.gz", nil
	case strings.HasSuffix(p, ".tar"):
		return tarArchiveCodec{gzip: false}, "tar", nil
	default:
		return nil, "", fmt.Errorf("unsupported archive extension for %q (supported: .zip, .tar, .tar.gz, .tgz)", sourcePath)
	}
}

func normalizeArchiveFormat(format string) string {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "zip":
		return "zip"
	case "tar":
		return "tar"
	case "tar.gz", "tgz":
		return "tar.gz"
	default:
		return strings.ToLower(strings.TrimSpace(format))
	}
}

func sanitizeArchiveEntryName(name string) (string, error) {
	raw := strings.ReplaceAll(strings.TrimSpace(name), "\\", "/")
	if raw == "" {
		return "", fmt.Errorf("empty archive entry name")
	}
	if strings.HasPrefix(raw, "/") {
		return "", fmt.Errorf("absolute archive entry path %q is not allowed", name)
	}
	parts := strings.Split(raw, "/")
	cleanParts := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" || part == "." {
			continue
		}
		if part == ".." {
			return "", fmt.Errorf("archive entry path traversal is not allowed: %q", name)
		}
		cleanParts = append(cleanParts, part)
	}
	if len(cleanParts) == 0 {
		return "", fmt.Errorf("invalid archive entry name %q", name)
	}
	return path.Join(cleanParts...), nil
}

type zipArchiveCodec struct{}

func (zipArchiveCodec) Write(files []archiveSourceFile, includeMetadata bool) ([]byte, int64, error) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	total := int64(0)
	for _, file := range files {
		name, err := sanitizeArchiveEntryName(file.Name)
		if err != nil {
			_ = zw.Close()
			return nil, 0, err
		}
		hdr := &zip.FileHeader{
			Name:   name,
			Method: zip.Deflate,
		}
		if includeMetadata && file.Mtime != nil {
			hdr.Modified = file.Mtime.UTC()
		} else {
			hdr.Modified = time.Unix(0, 0).UTC()
		}
		w, err := zw.CreateHeader(hdr)
		if err != nil {
			_ = zw.Close()
			return nil, 0, err
		}
		if _, err := w.Write(file.Data); err != nil {
			_ = zw.Close()
			return nil, 0, err
		}
		total += int64(len(file.Data))
	}
	if err := zw.Close(); err != nil {
		return nil, 0, err
	}
	return buf.Bytes(), total, nil
}

func (zipArchiveCodec) List(data []byte, limit int) (archiveListResult, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return archiveListResult{}, err
	}
	if limit <= 0 {
		limit = 200
	}
	result := archiveListResult{
		Entries: make([]types.ArchiveEntry, 0, min(limit, len(zr.File))),
	}
	for i, file := range zr.File {
		name, err := sanitizeArchiveEntryName(file.Name)
		if err != nil {
			return archiveListResult{}, err
		}
		size := int64(file.UncompressedSize64)
		if !file.FileInfo().IsDir() {
			result.TotalSizeBytes += size
		}
		if i >= limit {
			result.Truncated = true
			continue
		}
		entry := types.ArchiveEntry{
			Name:      name,
			IsDir:     file.FileInfo().IsDir(),
			SizeBytes: size,
		}
		if !file.Modified.IsZero() {
			ts := file.Modified.Unix()
			entry.Mtime = &ts
		}
		result.Entries = append(result.Entries, entry)
	}
	return result, nil
}

func (zipArchiveCodec) Decode(data []byte) ([]archiveDecodedEntry, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, err
	}
	out := make([]archiveDecodedEntry, 0, len(zr.File))
	for _, file := range zr.File {
		name, err := sanitizeArchiveEntryName(file.Name)
		if err != nil {
			return nil, err
		}
		entry := archiveDecodedEntry{
			Name:      name,
			IsDir:     file.FileInfo().IsDir(),
			SizeBytes: int64(file.UncompressedSize64),
		}
		if !file.Modified.IsZero() {
			ts := file.Modified.Unix()
			entry.Mtime = &ts
		}
		if !entry.IsDir {
			rc, err := file.Open()
			if err != nil {
				return nil, err
			}
			b, err := io.ReadAll(rc)
			_ = rc.Close()
			if err != nil {
				return nil, err
			}
			entry.Data = b
		}
		out = append(out, entry)
	}
	return out, nil
}

type tarArchiveCodec struct {
	gzip bool
}

func (c tarArchiveCodec) Write(files []archiveSourceFile, includeMetadata bool) ([]byte, int64, error) {
	var buf bytes.Buffer
	var tw *tar.Writer
	total := int64(0)

	if c.gzip {
		gw := gzip.NewWriter(&buf)
		tw = tar.NewWriter(gw)
		for _, file := range files {
			name, err := sanitizeArchiveEntryName(file.Name)
			if err != nil {
				_ = tw.Close()
				_ = gw.Close()
				return nil, 0, err
			}
			modTime := time.Unix(0, 0).UTC()
			if includeMetadata && file.Mtime != nil {
				modTime = file.Mtime.UTC()
			}
			h := &tar.Header{
				Name:     name,
				Mode:     0o644,
				Size:     int64(len(file.Data)),
				ModTime:  modTime,
				Typeflag: tar.TypeReg,
			}
			if err := tw.WriteHeader(h); err != nil {
				_ = tw.Close()
				_ = gw.Close()
				return nil, 0, err
			}
			if _, err := tw.Write(file.Data); err != nil {
				_ = tw.Close()
				_ = gw.Close()
				return nil, 0, err
			}
			total += int64(len(file.Data))
		}
		if err := tw.Close(); err != nil {
			_ = gw.Close()
			return nil, 0, err
		}
		if err := gw.Close(); err != nil {
			return nil, 0, err
		}
		return buf.Bytes(), total, nil
	}

	tw = tar.NewWriter(&buf)
	for _, file := range files {
		name, err := sanitizeArchiveEntryName(file.Name)
		if err != nil {
			_ = tw.Close()
			return nil, 0, err
		}
		modTime := time.Unix(0, 0).UTC()
		if includeMetadata && file.Mtime != nil {
			modTime = file.Mtime.UTC()
		}
		h := &tar.Header{
			Name:     name,
			Mode:     0o644,
			Size:     int64(len(file.Data)),
			ModTime:  modTime,
			Typeflag: tar.TypeReg,
		}
		if err := tw.WriteHeader(h); err != nil {
			_ = tw.Close()
			return nil, 0, err
		}
		if _, err := tw.Write(file.Data); err != nil {
			_ = tw.Close()
			return nil, 0, err
		}
		total += int64(len(file.Data))
	}
	if err := tw.Close(); err != nil {
		return nil, 0, err
	}
	return buf.Bytes(), total, nil
}

func (c tarArchiveCodec) List(data []byte, limit int) (archiveListResult, error) {
	entries, err := c.decodeAll(data)
	if err != nil {
		return archiveListResult{}, err
	}
	if limit <= 0 {
		limit = 200
	}
	result := archiveListResult{
		Entries: make([]types.ArchiveEntry, 0, min(limit, len(entries))),
	}
	for i, entry := range entries {
		if !entry.IsDir {
			result.TotalSizeBytes += entry.SizeBytes
		}
		if i >= limit {
			result.Truncated = true
			continue
		}
		result.Entries = append(result.Entries, types.ArchiveEntry{
			Name:      entry.Name,
			IsDir:     entry.IsDir,
			SizeBytes: entry.SizeBytes,
			Mtime:     entry.Mtime,
		})
	}
	return result, nil
}

func (c tarArchiveCodec) Decode(data []byte) ([]archiveDecodedEntry, error) {
	return c.decodeAll(data)
}

func (c tarArchiveCodec) decodeAll(data []byte) ([]archiveDecodedEntry, error) {
	var reader io.Reader = bytes.NewReader(data)
	if c.gzip {
		gr, err := gzip.NewReader(reader)
		if err != nil {
			return nil, err
		}
		defer gr.Close()
		reader = gr
	}
	tr := tar.NewReader(reader)
	out := make([]archiveDecodedEntry, 0)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		name, err := sanitizeArchiveEntryName(hdr.Name)
		if err != nil {
			return nil, err
		}
		entry := archiveDecodedEntry{
			Name:      name,
			IsDir:     hdr.FileInfo().IsDir(),
			SizeBytes: hdr.Size,
		}
		if !hdr.ModTime.IsZero() {
			ts := hdr.ModTime.Unix()
			entry.Mtime = &ts
		}
		if !entry.IsDir {
			b, err := io.ReadAll(tr)
			if err != nil {
				return nil, err
			}
			entry.Data = b
		}
		out = append(out, entry)
	}
	return out, nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
