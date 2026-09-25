package main

import (
    "encoding/json"
    "fmt"
    "io"
    "log"
    "mime/multipart"
    "net/http"
    "os"
    "os/exec"
    "path/filepath"
    "strings"
    "time"
)

const maxUpload = 50 << 20 // 50 MiB

func main() {
    mux := http.NewServeMux()
    mux.HandleFunc("/health", health)
    mux.HandleFunc("/convert", convert)

    port := os.Getenv("PORT")
    if port == "" { port = "10000" }

    srv := &http.Server{Addr: ":" + port, Handler: cors(mux), ReadHeaderTimeout: 15 * time.Second}
    log.Printf("wav-to-wem API listening on :%s", port)
    log.Fatal(srv.ListenAndServe())
}

func health(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodGet { http.Error(w, "method not allowed", http.StatusMethodNotAllowed); return }
    writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func convert(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodPost { http.Error(w, "method not allowed", http.StatusMethodNotAllowed); return }
    r.Body = http.MaxBytesReader(w, r.Body, maxUpload+1<<20)

    file, header, err := r.FormFile("file")
    if err != nil { writeJSON(w, http.StatusBadRequest, map[string]string{"error":"missing multipart field 'file'"}); return }
    defer file.Close()

    if header.Size > maxUpload { writeJSON(w, http.StatusRequestEntityTooLarge, map[string]string{"error":"file is larger than 50 MB"}); return }
    if !strings.EqualFold(filepath.Ext(header.Filename), ".wav") { writeJSON(w, http.StatusBadRequest, map[string]string{"error":"only .wav files are accepted"}); return }

    dir, err := os.MkdirTemp("", "wav2wem-api-")
    if err != nil { http.Error(w, "failed to create temp directory", http.StatusInternalServerError); return }
    defer os.RemoveAll(dir)

    input := filepath.Join(dir, "input.wav")
    output := filepath.Join(dir, "output.wem")
    if err := saveUpload(file, input); err != nil { http.Error(w, "failed to save upload", http.StatusInternalServerError); return }

    // Run the ENTIRE Windows wav2wem executable through Wine.
    // This is intentional: wav2wem launches its bundled oggenc2.exe internally.
    cmd := exec.Command("wine", "/opt/wav2wem/wav2wem.exe", input, "-o", output, "-q", "4")
    cmd.Env = append(os.Environ(), "WINEDEBUG=-all", "WINEPREFIX=/tmp/wineprefix")
    combined, err := cmd.CombinedOutput()
    log.Printf("wav2wem command output: %s", strings.TrimSpace(string(combined)))
    if err != nil {
        detail := strings.TrimSpace(string(combined))
        if detail == "" { detail = err.Error() }
        log.Printf("wav2wem error: %v: %s", err, detail)
        writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error":"WAV to WEM conversion failed", "detail": detail})
        return
    }

    info, err := os.Stat(output)
    if err != nil || info.Size() == 0 { writeJSON(w, http.StatusInternalServerError, map[string]string{"error":"converter completed but no WEM was produced"}); return }

    data, err := os.Open(output)
    if err != nil { http.Error(w, "failed to open WEM output", http.StatusInternalServerError); return }
    defer data.Close()

    base := strings.TrimSuffix(filepath.Base(header.Filename), filepath.Ext(header.Filename))
    downloadName := base + ".wem"
    w.Header().Set("Content-Type", "application/octet-stream")
    w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, sanitizeFilename(downloadName)))
    w.Header().Set("Content-Length", fmt.Sprintf("%d", info.Size()))
    w.WriteHeader(http.StatusOK)
    _, _ = io.Copy(w, data)
}

func saveUpload(src multipart.File, dst string) error {
    f, err := os.Create(dst)
    if err != nil { return err }
    defer f.Close()
    _, err = io.Copy(f, src)
    return err
}

func sanitizeFilename(s string) string {
    s = strings.ReplaceAll(s, `\`, "_")
    s = strings.ReplaceAll(s, `"`, "_")
    s = strings.ReplaceAll(s, "\r", "_")
    s = strings.ReplaceAll(s, "\n", "_")
    return s
}

func cors(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Access-Control-Allow-Origin", "*")
        w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS")
        w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
        if r.Method == http.MethodOptions { w.WriteHeader(http.StatusNoContent); return }
        next.ServeHTTP(w, r)
    })
}

func writeJSON(w http.ResponseWriter, status int, v any) {
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(status)
    _ = json.NewEncoder(w).Encode(v)
}
