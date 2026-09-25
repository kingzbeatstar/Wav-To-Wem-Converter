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
	port := os.Getenv("PORT")
	if port == "" {
		port = "10000"
	}

	http.HandleFunc("/health", health)
	http.HandleFunc("/convert", convert)

	log.Printf("WAV to WEM API listening on :%s", port)
	log.Fatal(http.ListenAndServe(":"+port, withCORS(http.DefaultServeMux)))
}

func health(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func convert(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxUpload+1<<20)

	file, header, err := r.FormFile("file")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "missing multipart file field named 'file'",
		})
		return
	}
	defer file.Close()

	if header.Size > maxUpload {
		writeJSON(w, http.StatusRequestEntityTooLarge, map[string]string{
			"error": "file is larger than 50 MB",
		})
		return
	}

	ext := strings.ToLower(filepath.Ext(header.Filename))
	if ext != ".wav" {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "only .wav files are supported",
		})
		return
	}

	workDir, err := os.MkdirTemp("", "wav2wem-api-*")
	if err != nil {
		http.Error(w, "could not create temporary directory", http.StatusInternalServerError)
		return
	}
	defer os.RemoveAll(workDir)

	input := filepath.Join(workDir, "input.wav")
	output := filepath.Join(workDir, "output.wem")

	if err := saveUpload(input, file); err != nil {
		log.Printf("upload error: %v", err)
		http.Error(w, "could not save uploaded WAV", http.StatusInternalServerError)
		return
	}

	// wav2wem embeds oggenc2.exe, so the converter is built for Windows
	// and executed through Wine inside this Linux container.
	converter := "/usr/local/bin/wav2wem.exe"
	cmd := exec.Command(
		"wine",
		converter,
		input,
		"-o", output,
		"-q", "4",
	)

	cmd.Env = append(os.Environ(),
		"WINEDEBUG=-all",
		"WINEPREFIX=/home/appuser/.wine",
		"HOME=/home/appuser",
	)

	start := time.Now()
	log.Printf("converting %q", header.Filename)

	combined, err := cmd.CombinedOutput()
	if err != nil {
		detail := strings.TrimSpace(string(combined))
		log.Printf("wav2wem error after %s: %v: %s", time.Since(start).Round(time.Millisecond), err, detail)

		// Return a useful diagnostic during deployment/testing. The frontend
		// can show this message instead of only "conversion failed".
		msg := "WAV to WEM conversion failed."
		if detail != "" {
			msg += " " + detail
		}
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": msg})
		return
	}

	log.Printf("conversion complete in %s", time.Since(start).Round(time.Millisecond))

	info, err := os.Stat(output)
	if err != nil || info.Size() == 0 {
		log.Printf("converter reported success but output is missing/empty")
		http.Error(w, "converter produced no WEM output", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`,
		safeDownloadName(strings.TrimSuffix(filepath.Base(header.Filename), filepath.Ext(header.Filename))+".wem")))
	http.ServeFile(w, r, output)
}

func saveUpload(dst string, src multipart.File) error {
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	limited := io.LimitReader(src, maxUpload+1)
	n, err := io.Copy(out, limited)
	if err != nil {
		return err
	}
	if n > maxUpload {
		return fmt.Errorf("upload exceeds limit")
	}
	return nil
}

func safeDownloadName(name string) string {
	name = strings.ReplaceAll(name, `"`, "")
	name = strings.ReplaceAll(name, "\r", "")
	name = strings.ReplaceAll(name, "\n", "")
	if name == "" || name == ".wem" {
		return "converted.wem"
	}
	return name
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "POST, GET, HEAD, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
