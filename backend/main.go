package main

import (
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const maxUpload = 50 * 1024 * 1024

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "10000"
	}

	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	})
	http.HandleFunc("/convert", convertHandler)

	log.Printf("WAV→WEM API listening on :%s", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

func convertHandler(w http.ResponseWriter, r *http.Request) {
	setCORS(w)

	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "POST a WAV file to /convert", http.StatusMethodNotAllowed)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxUpload)
	if err := r.ParseMultipartForm(maxUpload); err != nil {
		http.Error(w, "Upload is too large or invalid.", http.StatusRequestEntityTooLarge)
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "Missing form field named 'file'.", http.StatusBadRequest)
		return
	}
	defer file.Close()

	if !strings.EqualFold(filepath.Ext(header.Filename), ".wav") {
		http.Error(w, "Only .wav files are accepted.", http.StatusBadRequest)
		return
	}

	workDir, err := os.MkdirTemp("", "wav2wem-*")
	if err != nil {
		http.Error(w, "Could not create temporary workspace.", http.StatusInternalServerError)
		return
	}
	defer os.RemoveAll(workDir)

	input := filepath.Join(workDir, "input.wav")
	output := filepath.Join(workDir, "output.wem")

	dst, err := os.Create(input)
	if err != nil {
		http.Error(w, "Could not create temporary input file.", http.StatusInternalServerError)
		return
	}
	_, copyErr := io.Copy(dst, file)
	closeErr := dst.Close()
	if copyErr != nil || closeErr != nil {
		http.Error(w, "Could not save uploaded WAV.", http.StatusInternalServerError)
		return
	}

	// wav2wem's default quality is q4 (about 57 kbps) and it creates
	// modern Wwise Vorbis WEM using the embedded aoTuV 6.03 codebooks.
	cmd := exec.CommandContext(r.Context(), "/usr/local/bin/wav2wem",
		input, "-o", output, "-q", "4")
	cmd.Dir = workDir

	if combined, err := cmd.CombinedOutput(); err != nil {
		log.Printf("wav2wem error: %v: %s", err, strings.TrimSpace(string(combined)))
		http.Error(w, "WAV to WEM conversion failed.", http.StatusUnprocessableEntity)
		return
	}

	out, err := os.Open(output)
	if err != nil {
		http.Error(w, "Could not open converted WEM.", http.StatusInternalServerError)
		return
	}
	defer out.Close()

	info, err := out.Stat()
	if err != nil {
		http.Error(w, "Could not inspect converted WEM.", http.StatusInternalServerError)
		return
	}

	base := strings.TrimSuffix(filepath.Base(header.Filename), filepath.Ext(header.Filename))
	outName := safeFilename(base) + ".wem"

	w.Header().Set("Content-Type", mime.TypeByExtension(".wem"))
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, outName))
	w.Header().Set("Content-Length", fmt.Sprintf("%d", info.Size()))
	w.Header().Set("Cache-Control", "no-store")
	http.ServeContent(w, r, outName, time.Now(), out)
}

func setCORS(w http.ResponseWriter) {
	// Set this to your exact site origin after the first successful deployment.
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	w.Header().Set("Access-Control-Expose-Headers", "Content-Disposition")
}

func safeFilename(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "converted"
	}
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z',
			r >= 'A' && r <= 'Z',
			r >= '0' && r <= '9',
			r == '-', r == '_', r == ' ', r == '.':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	return strings.TrimSpace(b.String())
}
