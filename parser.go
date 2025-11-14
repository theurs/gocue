package gocue

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// Parse читает и разбирает CUE sheet из предоставленного io.Reader.
// В случае успеха возвращает указатель на полностью заполненную структуру Cuesheet.
// В случае ошибки возвращает nil и ошибку, описывающую проблему.
func Parse(r io.Reader) (*Cuesheet, error) {
	sheet := &Cuesheet{}
	scanner := bufio.NewScanner(r)

	var currentFile *File
	var currentTrack *Track
	var pendingIndices []Index // <-- ИЗМЕНЕНИЕ: Буфер для "опережающих" индексов
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue // Пропускаем пустые строки
		}

		parts, err := smartSplit(line)
		if err != nil {
			return nil, fmt.Errorf("line %d: invalid quoting: %w", lineNum, err)
		}

		if len(parts) == 0 {
			continue
		}

		command := strings.ToUpper(parts[0])
		args := parts[1:]

		switch command {
		case "REM":
			if len(args) > 0 {
				sheet.Rem = append(sheet.Rem, strings.Join(args, " "))
			}
		case "CATALOG":
			if len(args) < 1 {
				return nil, fmt.Errorf("line %d: CATALOG command requires an argument", lineNum)
			}
			sheet.Catalog = args[0]
		case "CDTEXTFILE":
			if len(args) < 1 {
				return nil, fmt.Errorf("line %d: CDTEXTFILE command requires an argument", lineNum)
			}
			sheet.CDTextFile = args[0]
		case "TITLE":
			if len(args) < 1 {
				return nil, fmt.Errorf("line %d: TITLE command requires an argument", lineNum)
			}
			title := args[0]
			if currentTrack != nil {
				currentTrack.Title = title
			} else { // Глобальный контекст
				sheet.Title = title
			}
		case "PERFORMER":
			if len(args) < 1 {
				return nil, fmt.Errorf("line %d: PERFORMER command requires an argument", lineNum)
			}
			performer := args[0]
			if currentTrack != nil {
				currentTrack.Performer = performer
			} else { // Глобальный контекст
				sheet.Performer = performer
			}
		case "SONGWRITER":
			if len(args) < 1 {
				return nil, fmt.Errorf("line %d: SONGWRITER command requires an argument", lineNum)
			}
			songwriter := args[0]
			if currentTrack != nil {
				currentTrack.Songwriter = songwriter
			} else { // Глобальный контекст
				sheet.Songwriter = songwriter
			}
		case "FILE":
			if len(args) < 2 {
				return nil, fmt.Errorf("line %d: FILE command requires name and type arguments", lineNum)
			}
			file := &File{Name: args[0], Type: strings.ToUpper(args[1])}
			sheet.Files = append(sheet.Files, file)
			currentFile = file
			currentTrack = nil   // Сбрасываем контекст трека при объявлении нового файла
			pendingIndices = nil // ИЗМЕНЕНИЕ: Очищаем буфер индексов
		case "TRACK":
			if currentFile == nil {
				return nil, fmt.Errorf("line %d: TRACK command found outside of a FILE context", lineNum)
			}
			if len(args) < 2 {
				return nil, fmt.Errorf("line %d: TRACK command requires number and type arguments", lineNum)
			}
			num, err := strconv.Atoi(args[0])
			if err != nil {
				return nil, fmt.Errorf("line %d: invalid track number: %w", lineNum, err)
			}
			track := &Track{Number: num, Type: strings.ToUpper(args[1])}
			// ИЗМЕНЕНИЕ: Присоединяем накопленные индексы к новому треку
			if len(pendingIndices) > 0 {
				track.Indices = append(track.Indices, pendingIndices...)
				pendingIndices = nil // Очищаем буфер
			}
			currentFile.Tracks = append(currentFile.Tracks, track)
			currentTrack = track
		case "INDEX":
			if len(args) < 2 {
				return nil, fmt.Errorf("line %d: INDEX command requires number and timecode arguments", lineNum)
			}
			num, err := strconv.Atoi(args[0])
			if err != nil {
				return nil, fmt.Errorf("line %d: invalid index number: %w", lineNum, err)
			}
			timecode, err := parseTimecode(args[1])
			if err != nil {
				return nil, fmt.Errorf("line %d: invalid timecode for INDEX: %w", lineNum, err)
			}
			index := Index{Number: num, Time: timecode}

			// ИЗМЕНЕНИЕ: Главная логика исправления
			if currentTrack != nil {
				// Если контекст трека уже есть, добавляем как обычно
				currentTrack.Indices = append(currentTrack.Indices, index)
			} else if currentFile != nil {
				// Если есть контекст файла, но нет трека, добавляем в буфер
				pendingIndices = append(pendingIndices, index)
			} else {
				// Если нет даже контекста файла, это ошибка
				return nil, fmt.Errorf("line %d: INDEX command found outside of a FILE context", lineNum)
			}
		case "PREGAP":
			if currentTrack == nil {
				return nil, fmt.Errorf("line %d: PREGAP command found outside of a TRACK context", lineNum)
			}
			if len(args) < 1 {
				return nil, fmt.Errorf("line %d: PREGAP command requires a timecode argument", lineNum)
			}
			timecode, err := parseTimecode(args[0])
			if err != nil {
				return nil, fmt.Errorf("line %d: invalid timecode for PREGAP: %w", lineNum, err)
			}
			currentTrack.Pregap = timecode
		case "POSTGAP":
			if currentTrack == nil {
				return nil, fmt.Errorf("line %d: POSTGAP command found outside of a TRACK context", lineNum)
			}
			if len(args) < 1 {
				return nil, fmt.Errorf("line %d: POSTGAP command requires a timecode argument", lineNum)
			}
			timecode, err := parseTimecode(args[0])
			if err != nil {
				return nil, fmt.Errorf("line %d: invalid timecode for POSTGAP: %w", lineNum, err)
			}
			currentTrack.Postgap = timecode
		case "FLAGS":
			if currentTrack == nil {
				return nil, fmt.Errorf("line %d: FLAGS command found outside of a TRACK context", lineNum)
			}
			currentTrack.Flags = append(currentTrack.Flags, args...)
		case "ISRC":
			if currentTrack == nil {
				return nil, fmt.Errorf("line %d: ISRC command found outside of a TRACK context", lineNum)
			}
			if len(args) < 1 {
				return nil, fmt.Errorf("line %d: ISRC command requires an argument", lineNum)
			}
			currentTrack.ISRC = args[0]
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading input: %w", err)
	}

	// Финальный шаг: проходим по созданным структурам и устанавливаем
	// внутренние ссылки на родительские элементы. Это нужно для работы
	// методов вроде track.Duration().
	for _, f := range sheet.Files {
		f.parentSheet = sheet
		for _, t := range f.Tracks {
			t.parentFile = f
		}
	}

	return sheet, nil
}

// parseTimecode разбирает строку формата "MM:SS:FF" в структуру Timecode.
func parseTimecode(s string) (Timecode, error) {
	parts := strings.Split(s, ":")
	if len(parts) != 3 {
		return Timecode{}, errors.New("timecode must be in MM:SS:FF format")
	}

	minutes, err := strconv.Atoi(parts[0])
	if err != nil {
		return Timecode{}, fmt.Errorf("invalid minutes value: %s", parts[0])
	}

	seconds, err := strconv.Atoi(parts[1])
	if err != nil {
		return Timecode{}, fmt.Errorf("invalid seconds value: %s", parts[1])
	}
	if seconds > 59 {
		return Timecode{}, fmt.Errorf("seconds value cannot exceed 59: %d", seconds)
	}

	frames, err := strconv.Atoi(parts[2])
	if err != nil {
		return Timecode{}, fmt.Errorf("invalid frames value: %s", parts[2])
	}
	if frames >= FramesPerSecond {
		return Timecode{}, fmt.Errorf("frames value must be less than %d: %d", FramesPerSecond, frames)
	}

	return Timecode{Minutes: minutes, Seconds: seconds, Frames: frames}, nil
}

// smartSplit разделяет строку на части, учитывая двойные кавычки.
// Аргументы в кавычках считаются единым целым.
func smartSplit(line string) ([]string, error) {
	var result []string
	var current strings.Builder
	inQuote := false

	for i, r := range line {
		switch r {
		case '"':
			inQuote = !inQuote
		case ' ', '\t':
			if inQuote {
				current.WriteRune(r)
			} else {
				if current.Len() > 0 {
					result = append(result, current.String())
					current.Reset()
				}
			}
		default:
			// Команда не может быть в кавычках
			if len(result) == 0 && current.Len() == 0 && r == '"' {
				return nil, errors.New("command cannot be quoted")
			}
			current.WriteRune(r)
		}
		// Проверка на незакрытую кавычку в конце строки
		if i == len(line)-1 && inQuote {
			return nil, errors.New("mismatched quotes")
		}
	}

	if current.Len() > 0 {
		result = append(result, current.String())
	}

	return result, nil
}
