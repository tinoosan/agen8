package glance

import (
    "fmt"
    "os"
    "path/filepath"
    "sort"
)

 type FileInfo struct {
    Path string
    Size int64
 }

 func ScanFiles(root string) ([]FileInfo, error) {
    var entries []FileInfo
    err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
        if err != nil {
            return fmt.Errorf("walk failed: %w", err)
        }
        if d.IsDir() {
            return nil
        }
        info, err := d.Info()
        if err != nil {
            return err
        }
        rel, err := filepath.Rel(root, path)
        if err != nil {
            return err
        }
        entries = append(entries, FileInfo{Path: rel, Size: info.Size()})
        return nil
    })
    if err != nil {
        return nil, err
    }
    sort.Slice(entries, func(i, j int) bool {
        return entries[i].Size > entries[j].Size
    })
    return entries, nil
}

 func TopN(entries []FileInfo, n int) []FileInfo {
    if len(entries) < n {
        return entries
    }
    return entries[:n]
}
