package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gocue" // Импортируем нашу библиотеку
)

// sanitizeFilename удаляет символы, недопустимые в именах файлов Windows/Linux.
func sanitizeFilename(name string) string {
	re := regexp.MustCompile(`[<>:"/\\|?*]`)
	sanitized := re.ReplaceAllString(name, "_")
	return strings.TrimSpace(sanitized)
}

// findActualAudioFile пытается найти реальный аудиофайл.
// Если файл, указанный в CUE, не существует, он ищет файлы с тем же
// именем, но с другими популярными lossless-расширениями.
func findActualAudioFile(cuePath, filenameFromCue string) (string, error) {
	basePath := filepath.Join(filepath.Dir(cuePath), filenameFromCue)

	// 1. Проверяем оригинальное имя файла
	if _, err := os.Stat(basePath); err == nil {
		return basePath, nil // Файл найден
	}

	// 2. Если не найден, пробуем другие расширения
	possibleExtensions := []string{".flac", ".ape", ".wv", ".tak"}
	baseNameWithoutExt := strings.TrimSuffix(filenameFromCue, filepath.Ext(filenameFromCue))

	for _, ext := range possibleExtensions {
		newFilename := baseNameWithoutExt + ext
		newPath := filepath.Join(filepath.Dir(cuePath), newFilename)
		if _, err := os.Stat(newPath); err == nil {
			log.Printf("INFO: Audio file '%s' not found, using '%s' instead.", filenameFromCue, newFilename)
			return newPath, nil
		}
	}

	return "", fmt.Errorf("audio file '%s' not found, and no alternatives could be found", filenameFromCue)
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: go run main.go <path/to/your.cue>")
		os.Exit(1)
	}
	cuePath := os.Args[1]

	f, err := os.Open(cuePath)
	if err != nil {
		log.Fatalf("FATAL: Failed to open CUE file '%s': %v", cuePath, err)
	}
	defer f.Close()

	sheet, err := gocue.Parse(f)
	if err != nil {
		log.Fatalf("FATAL: Failed to parse CUE file: %v", err)
	}

	fmt.Printf("# CUE sheet: %s - %s\n", sheet.Performer, sheet.Title)
	fmt.Println("# Generated ffmpeg commands:")
	fmt.Println()

	for _, file := range sheet.Files {
		sourceAudioPath, err := findActualAudioFile(cuePath, file.Name)
		if err != nil {
			log.Printf("WARN: Skipping file block for '%s' because the audio file could not be found: %v", file.Name, err)
			continue
		}

		for _, track := range file.Tracks {
			startTime := track.StartTime().AsDuration()
			duration := track.Duration()

			outputFileName := fmt.Sprintf("%02d - %s.ogg", track.Number, track.Title)
			outputFileName = sanitizeFilename(outputFileName)

			var cmd string
			baseCmd := fmt.Sprintf(
				`ffmpeg -i "%s" -ss %f -vn -map_metadata -1`,
				sourceAudioPath,
				startTime.Seconds(),
			)

			if duration > 0 {
				cmd = fmt.Sprintf(
					`%s -t %f -c:a libvorbis -q:a 5 "%s"`,
					baseCmd,
					duration.Seconds(),
					outputFileName,
				)
			} else {
				cmd = fmt.Sprintf(
					`%s -c:a libvorbis -q:a 5 "%s"`,
					baseCmd,
					outputFileName,
				)
			}

			metadataCmd := fmt.Sprintf(
				` -metadata artist="%s" -metadata album_artist="%s" -metadata album="%s" -metadata title="%s" -metadata track="%d"`,
				track.Performer,
				sheet.Performer,
				sheet.Title,
				track.Title,
				track.Number,
			)

			fmt.Println(cmd + metadataCmd)
		}
	}
}
