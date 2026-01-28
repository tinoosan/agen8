package glance

import (
    "encoding/json"
    "fmt"
    "io"
)

 func FormatTable(entries []FileInfo, w io.Writer) error {
    for _, entry := range entries {
        _, err := fmt.Fprintf(w, "%12d %s\n", entry.Size, entry.Path)
        if err != nil {
            return err
        }
    }
    return nil
}

 func FormatJSON(entries []FileInfo, w io.Writer) error {
    enc := json.NewEncoder(w)
    enc.SetIndent("", "  ")
    return enc.Encode(entries)
}
