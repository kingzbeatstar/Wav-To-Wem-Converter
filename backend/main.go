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
const converter = "/opt/wav2wem/wav2wem.exe"
const wineHome = "/home/appuser"
const winePrefix = "/home/appuser/.wine"

func main() {
	log.Printf("wav-to-wem API starting")
	log.Printf("converter: Windows wav2wem.exe v0.1 via Wine")
	log.Printf("converter path: %s", converter)
	log.Printf("HOME=%s WINEPREFIX=%s", wineHome, winePrefix)

	if out, err := runWine("--version"); err != nil {
		log.Fatalf("WINE SELF-TEST FAILED: %v: %s", err, strings.TrimSpace(string(out)))
	} else {
		log.Printf("WINE SELF-TEST OK: %s", strings.TrimSpace(string(out)))
	}

	if out, err := runWine("wineboot", "--init"); err != nil {
		log.Fatalf("WINE PREFIX INIT FAILED: %v: %s", err, strings.TrimSpace(string(out)))
	} else {
		log.Printf("WINE PREFIX INIT OK")
	}

	if !fileExists(converter) {
		log.Fatalf("converter missing: %s", converter)
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
		"version": "v7",
	})
}

func health(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "version": "v7"})
}

func diagnostics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	result := map[string]any{
		"version":           "v7",
		"wine_home":         wineHome,
		"wine_prefix":       winePrefix,
		"converter":         converter,
		"converter_present": fileExists(converter),
		"prefix_present":    fileExists(winePrefix),
	}
	wineOut, wineErr := runWine("--version")
	result["wine_available"] = wineErr == nil
	result["wine_output"] = strings.TrimSpace(string(wineOut))
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

	if !fileExists(converter) {
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error":  "converter is missing",
			"detail": converter,
		})
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

	wineInput := winePath(input)
	wineOutput := winePath(output)
	args := []string{converter, wineInput, "-o", wineOutput, "-q", "4"}

	log.Printf("starting conversion: %s", header.Filename)
	log.Printf("command: wine %s", strings.Join(args[1:], " "))
	log.Printf("conversion environment: HOME=%s WINEPREFIX=%s", wineHome, winePrefix)

	start := time.Now()
	cmd := exec.Command("wine", args...)
	cmd.Env = wineEnv()
	out, err := cmd.CombinedOutput()
	elapsed := time.Since(start)
	cleanOut := strings.TrimSpace(string(out))
	if cleanOut != "" {
		log.Printf("wav2wem output: %s", cleanOut)
	}
	if err != nil {
		log.Printf("wav2wem failed after %s: %v", elapsed.Round(time.Millisecond), err)
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{
			"error":  "WAV to WEM conversion failed",
			"detail": cleanOut,
		})
		return
	}
	log.Printf("conversion succeeded in %s", elapsed.Round(time.Millisecond))

	info, err := os.Stat(output)
	if err != nil || info.Size() == 0 {
		detail := "converter completed but no WEM was produced"
		if err != nil {
			detail += ": " + err.Error()
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error":  "WEM output missing",
			"detail": detail,
		})
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

func wineEnv() []string {
	// Build a clean environment and REMOVE any inherited HOME/WINEPREFIX/TMPDIR
	// values before adding our own. Appending duplicate variables is unsafe:
	// Wine may read the earlier HOME=/tmp value and reject /tmp as its prefix.
	keysToReplace := map[string]bool{
		"HOME": true, "WINEPREFIX": true, "WINEARCH": true,
		"WINEDEBUG": true, "TMPDIR": true,
	}
	env := make([]string, 0, len(os.Environ())+5)
	for _, item := range os.Environ() {
		key := item
		if i := strings.IndexByte(item, '='); i >= 0 {
			key = item[:i]
		}
		if keysToReplace[key] {
			continue
		}
		env = append(env, item)
	}
	env = append(env,
		"HOME="+wineHome,
		"WINEPREFIX="+winePrefix,
		"WINEARCH=win64",
		"WINEDEBUG=-all",
		"TMPDIR=/tmp",
	)
	return env
}

func runWine(args ...string) ([]byte, error) {
	cmd := exec.Command("wine", args...)
	cmd.Env = wineEnv()
	return cmd.CombinedOutput()
}

func winePath(p string) string {
	p = filepath.ToSlash(filepath.Clean(p))
	return "Z:" + strings.ReplaceAll(p, "/", `\`)
}

func fileExists(p string) bool {
	info, err := os.Stat(p)
	return err == nil && !info.IsDir()
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
