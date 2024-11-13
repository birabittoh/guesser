package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

func saveUploadedFile(file io.Reader, path string) error {
	out, err := os.Create(path)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, file)
	return err
}

func processHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Metodo non supportato", http.StatusMethodNotAllowed)
		return
	}

	// Parse del form per ottenere i file e gli altri parametri
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		http.Error(w, "Errore parsing form", http.StatusBadRequest)
		return
	}

	// Recupera i file audio e video
	audioFile, _, err := r.FormFile("audioFile")
	if err != nil {
		http.Error(w, "File audio mancante", http.StatusBadRequest)
		return
	}
	defer audioFile.Close()

	videoFile, _, err := r.FormFile("videoFile")
	if err != nil {
		http.Error(w, "File video mancante", http.StatusBadRequest)
		return
	}
	defer videoFile.Close()

	// Salva i file temporaneamente
	audioPath := "temp_audio.opus"
	videoPath := "temp_video.mp4"
	outputPath := "output.mp4"

	if err := saveUploadedFile(audioFile, audioPath); err != nil {
		http.Error(w, "Errore nel salvataggio del file audio", http.StatusInternalServerError)
		return
	}
	if err := saveUploadedFile(videoFile, videoPath); err != nil {
		http.Error(w, "Errore nel salvataggio del file video", http.StatusInternalServerError)
		return
	}

	// Recupera i parametri di configurazione dal form
	skip, _ := strconv.ParseFloat(r.FormValue("skip"), 64)
	fade, _ := strconv.ParseFloat(r.FormValue("fade"), 64)
	delay, _ := strconv.ParseFloat(r.FormValue("delay"), 64)
	duration, _ := strconv.ParseFloat(r.FormValue("duration"), 64)

	// Conversione delay in millisecondi
	delayMs := int(delay * 1000)
	fadeOutStart := duration - fade

	// Costruzione del filtro audio
	audioFilter := fmt.Sprintf("afade=t=out:st=%.2f:d=%.2f", fadeOutStart, fade)
	if skip >= fade {
		audioFilter = fmt.Sprintf("afade=t=in:ss=0:d=%.2f,%s", fade, audioFilter)
	}

	// Costruzione del comando ffmpeg
	ffmpegCommand := []string{
		"-i", videoPath,
		"-i", audioPath,
		"-filter_complex",
		fmt.Sprintf("[1:a]atrim=start=%.2f:end=%.2f,asetpts=PTS-STARTPTS,%s,adelay=%d|%d[song];[0:a][song]amix=inputs=2:duration=first[audio_mix];[0:v]copy[v]",
			skip, skip+duration, audioFilter, delayMs, delayMs),
		"-map", "[v]", "-map", "[audio_mix]",
		"-c:v", "libx264",
		"-c:a", "aac",
		"-shortest", outputPath,
		"-y",
	}

	// Esecuzione del comando ffmpeg
	cmd := exec.Command("ffmpeg", ffmpegCommand...)
	println(strings.Join(ffmpegCommand, " "))
	cmd.Stderr = os.Stderr // Mostra eventuali errori in console
	cmd.Stdout = os.Stdout

	if err := cmd.Run(); err != nil {
		http.Error(w, "Errore durante l'esecuzione di ffmpeg", http.StatusInternalServerError)
		return
	}

	// Impostazioni per scaricare il file di output
	w.Header().Set("Content-Disposition", "attachment; filename=output.mp4")
	w.Header().Set("Content-Type", "video/mp4")

	// Restituisce il file output.mp4 in risposta
	outputFile, err := os.Open(outputPath)
	if err != nil {
		http.Error(w, "Errore nell'apertura del file output", http.StatusInternalServerError)
		return
	}
	defer outputFile.Close()

	_, err = io.Copy(w, outputFile)
	if err != nil {
		http.Error(w, "Errore durante la copia del file", http.StatusInternalServerError)
		return
	}

	// Pulizia dei file temporanei
	os.Remove(audioPath)
	os.Remove(videoPath)
	os.Remove(outputPath)
}
