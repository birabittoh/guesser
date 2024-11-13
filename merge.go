package main

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strconv"
)

const (
	outputPath = "output.mp4"
	audioPath  = "temp_audio.opus"
	videoPath  = "temp_video.mp4"
)

var sem = make(chan struct{}, 1) // Semaforo con buffer 1

func saveUploadedFile(file io.Reader, path string) error {
	out, err := os.Create(path)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, file)
	return err
}

// Funzione helper per restituire un errore di richiesta non valida
func badRequest(w http.ResponseWriter, reason string) error {
	http.Error(w, reason, http.StatusBadRequest)
	return errors.New(reason)
}

// Funzione per leggere e validare i dati del form e salvare i file temporaneamente
func parseAndValidateForm(w http.ResponseWriter, r *http.Request) (skip, fade, delay, duration float64, err error) {
	if r.Method != http.MethodPost {
		err = badRequest(w, "Metodo non supportato")
		return
	}

	// Parsing del form
	if err = r.ParseMultipartForm(10 << 20); err != nil {
		err = badRequest(w, "Errore nel form")
		return
	}

	// Recupera i file audio e video
	audioFile, audioHeader, err := r.FormFile("audioFile")
	if err != nil {
		err = badRequest(w, "File audio mancante")
		return
	}
	defer audioFile.Close()

	videoFile, videoHeader, err := r.FormFile("videoFile")
	if err != nil {
		err = badRequest(w, "File video mancante")
		return
	}
	defer videoFile.Close()

	// Controllo delle dimensioni dei file
	const maxFileSize = 100 << 20 // 100 MB
	if audioHeader.Size > maxFileSize {
		err = badRequest(w, "File audio troppo grande")
		return
	}
	if videoHeader.Size > maxFileSize {
		err = badRequest(w, "File video troppo grande")
		return
	}

	// Recupera e valida i parametri dal form
	if skip, err = strconv.ParseFloat(r.FormValue("skip"), 64); err != nil || skip < 0 {
		err = badRequest(w, "Parametro skip non valido")
		return
	}

	if fade, err = strconv.ParseFloat(r.FormValue("fade"), 64); err != nil || fade < 0 {
		err = badRequest(w, "Parametro fade non valido")
		return
	}

	if delay, err = strconv.ParseFloat(r.FormValue("delay"), 64); err != nil || delay < 0 {
		err = badRequest(w, "Parametro delay non valido")
		return
	}

	if duration, err = strconv.ParseFloat(r.FormValue("duration"), 64); err != nil || duration <= 0 {
		err = badRequest(w, "Parametro duration non valido")
		return
	}

	// Salva i file temporaneamente
	if err = saveUploadedFile(audioFile, audioPath); err != nil {
		http.Error(w, "Errore nel salvataggio del file audio", http.StatusInternalServerError)
		return
	}
	if err = saveUploadedFile(videoFile, videoPath); err != nil {
		http.Error(w, "Errore nel salvataggio del file video", http.StatusInternalServerError)
		return
	}

	return
}

// Funzione per gestire il processamento multimediale
func processMedia(w http.ResponseWriter, skip, fade, delay, duration float64) error {
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
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("errore durante l'esecuzione di ffmpeg: %v", err)
	}

	// Impostazioni per scaricare il file di output
	w.Header().Set("Content-Disposition", "attachment; filename=output.mp4")
	w.Header().Set("Content-Type", "video/mp4")

	// Pulizia dei file temporanei
	os.Remove(audioPath)
	os.Remove(videoPath)

	return nil
}

// Handler principale
func processHandler(w http.ResponseWriter, r *http.Request) {
	sem <- struct{}{}
	defer func() { <-sem }() // Rilascia il semaforo alla fine

	skip, fade, delay, duration, err := parseAndValidateForm(w, r)
	if err != nil {
		return // Gli errori vengono già gestiti in parseAndValidateForm
	}

	if err := processMedia(w, skip, fade, delay, duration); err != nil {
		http.Error(w, "Errore durante il processamento multimediale", http.StatusInternalServerError)
		return
	}

	// Restituisce il file output.mp4 in risposta
	outputFile, err := os.Open(outputPath)
	if err != nil {
		return
	}
	defer outputFile.Close()

	if _, err = io.Copy(w, outputFile); err != nil {
		return
	}

	os.Remove(outputPath)
}
