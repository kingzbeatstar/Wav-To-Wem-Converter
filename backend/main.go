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

	wav2wem "github.com/pas2k/wav2wem"
)

const maxUpload = 50 << 20 // 50 MiB

func main() {
	log.Printf("wav-to-wem API starting")
	log.Printf("converter architecture: Linux API + patched wav2wem + Wine + embedded oggenc2.exe")

	// Verify Wine itself is callable at startup. We deliberately do not run a
	// full audio conversion here because the health check should stay cheap.
	if out, err := exec.Command("wine", "--version").CombinedOutput(); err != nil {
		log.Printf("WINE SELF-TEST FAILED: %v: %s", err, strings.TrimSpace(string(out)))
	} else {
		log.Printf("WINE SELF-TEST OK: %s", strings.TrimSpace(string(out)))
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", root)
	mux.HandleFunc("/health", health)
	mux.HandleFunc("/diagnostics", diagnostics)
	mux.HandleFunc("/convert", convert)

	port := os.Getenv("PORT")
	if port == "" {
		port = "10000"
	}

	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           cors(mux),
		ReadHeaderTimeout: 15 * time.Second,
	}
	log.Printf("wav-to-wem API listening on :%s", port)
	log.Fatal(srv.ListenAndServe())
}

func root(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"service": "wav-to-wem-converter",
		"ok":      true,
		"version": "v4",
	})
}

func health(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func diagnostics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	wineOut, wineErr := exec.Command("wine", "--version").CombinedOutput()
	result := map[string]any{
		"version":        "v4",
		"architecture":   "Linux API + patched wav2wem + Wine + embedded oggenc2.exe",
		"wine_available": wineErr == nil,
		"wine_output":    strings.TrimSpace(string(wineOut)),
	}
	if wineErr != nil {
		result["wine_error"] = wineErr.Error()
		writeJSON(w, http.StatusServiceUnavailable, result)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func convert(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxUpload+1<<20)

	file, header, err := r.FormFile("file")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing multipart field 'file'"})
		return
	}
	defer file.Close()

	if header.Size > maxUpload {
		writeJSON(w, http.StatusRequestEntityTooLarge, map[string]string{"error": "file is larger than 50 MB"})
		return
	}
	if !strings.EqualFold(filepath.Ext(header.Filename), ".wav") {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "only .wav files are accepted"})
		return
	}

	dir, err := os.MkdirTemp("", "wav2wem-api-")
	if err != nil {
		http.Error(w, "failed to create temp directory", http.StatusInternalServerError)
		return
	}
	defer os.RemoveAll(dir)

	input := filepath.Join(dir, "input.wav")
	output := filepath.Join(dir, "output.wem")
	if err := saveUpload(file, input); err != nil {
		http.Error(w, "failed to save upload", http.StatusInternalServerError)
		return
	}

	// This calls the patched upstream library directly. The only modification
	// is inside wav2wem/encoder.go, where its embedded oggenc2.exe is launched
	// through Wine instead of being exec'd by Linux as a native binary.
	log.Printf("starting conversion: %s", header.Filename)
	start := time.Now()
	enc := wav2wem.DefaultEncodeOptions() // quality 4
	wemOpts := wav2wem.Options{IncludeHash: true}
	err = wav2wem.ConvertFile(input, output, enc, wemOpts)
	elapsed := time.Since(start)
	if err != nil {
		log.Printf("wav2wem conversion failed after %s: %v", elapsed.Round(time.Millisecond), err)
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{
			"error":  "WAV to WEM conversion failed",
			"detail": err.Error(),
		})
		return
	}
	log.Printf("conversion succeeded in %s", elapsed.Round(time.Millisecond))

	info, err := os.Stat(output)
	if err != nil || info.Size() == 0 {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "converter completed but no WEM was produced"})
		return
	}

	data, err := os.Open(output)
	if err != nil {
		http.Error(w, "failed to open WEM output", http.StatusInternalServerError)
		return
	}
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
	if err != nil {
		return err
	}
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
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
