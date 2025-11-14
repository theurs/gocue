// Package gocue предоставляет структуры и функции для парсинга файлов CUE sheet.
// Он нацелен на полную поддержку спецификации, включая нестандартные расширения,
// используемые в популярных программах.
package gocue

import (
	"fmt"
	"time"
)

const (
	// FramesPerSecond определяет стандартное количество фреймов в секунде для Audio CD.
	FramesPerSecond = 75
)

// Timecode представляет время в формате MM:SS:FF (минуты:секунды:фреймы).
// Этот формат является стандартом для CUE sheet.
type Timecode struct {
	Minutes int
	Seconds int
	Frames  int
}

// String возвращает строковое представление таймкода в формате "MM:SS:FF".
func (t Timecode) String() string {
	return fmt.Sprintf("%02d:%02d:%02d", t.Minutes, t.Seconds, t.Frames)
}

// TotalFrames конвертирует время в общее количество фреймов.
// Это основной способ для выполнения арифметических операций с таймкодами.
func (t Timecode) TotalFrames() int {
	return (t.Minutes * 60 * FramesPerSecond) + (t.Seconds * FramesPerSecond) + t.Frames
}

// AsDuration конвертирует таймкод в стандартный тип time.Duration.
// Полезно для интеграции с другими частями стандартной библиотеки Go.
func (t Timecode) AsDuration() time.Duration {
	// Сначала умножаем на количество наносекунд в секунде, потом делим,
	// чтобы избежать потери точности при делении time.Second / FramesPerSecond.
	nanoseconds := (int64(t.TotalFrames()) * 1e9) / FramesPerSecond
	return time.Duration(nanoseconds)
}

// Index представляет команду INDEX в CUE-файле.
// Каждый трек должен иметь как минимум INDEX 01.
type Index struct {
	Number int      // Номер индекса (00-99).
	Time   Timecode // Временная метка индекса.
}

// Track представляет один трек (дорожку) на диске.
// Он содержит метаданные и временные метки.
type Track struct {
	Number     int
	Type       string
	Title      string
	Performer  string
	Songwriter string
	ISRC       string   // International Standard Recording Code.
	Flags      []string // Флаги субкодов (DCP, 4CH, PRE, SCMS).
	Indices    []Index  // Список всех индексов трека.
	Pregap     Timecode // Длительность предтрековой паузы.
	Postgap    Timecode // Длительность посттрековой паузы.

	// parentFile - внутренняя ссылка на родительский файл для вычислений.
	parentFile *File
}

// StartTime возвращает официальное время начала трека (время, указанное в INDEX 01).
// Если INDEX 01 не найден, возвращает нулевой таймкод.
func (t *Track) StartTime() Timecode {
	for _, idx := range t.Indices {
		if idx.Number == 1 {
			return idx.Time
		}
	}
	return Timecode{}
}

// Duration вычисляет длительность трека.
// Для последнего трека на диске длительность определить невозможно,
// поэтому будет возвращена нулевая длительность. В этом случае потребитель
// библиотеки должен сам решить, как обрабатывать конец файла (например, читать до EOF).
func (t *Track) Duration() time.Duration {
	if t.parentFile == nil {
		return 0
	}
	return t.parentFile.getTrackDuration(t.Number)
}

// File представляет команду FILE в CUE-файле.
// Он описывает один физический файл (например, .wav или .bin) и треки внутри него.
type File struct {
	Name   string
	Type   string   // Тип файла (WAVE, MP3, BINARY и т.д.).
	Tracks []*Track // Список треков, содержащихся в этом файле.

	// parentSheet - внутренняя ссылка на корневой объект.
	parentSheet *Cuesheet
}

// getTrackDuration ищет текущий и следующий трек для вычисления длительности.
// Эта логика вынесена на уровень File, так как трек сам по себе не знает о соседях.
func (f *File) getTrackDuration(trackNumber int) time.Duration {
	var currentTrack, nextTrack *Track

	// Ищем все треки по порядку во всем CUE sheet
	var allTracks []*Track
	if f.parentSheet != nil {
		for _, file := range f.parentSheet.Files {
			allTracks = append(allTracks, file.Tracks...)
		}
	} else {
		// Fallback, если родительский sheet не установлен
		allTracks = f.Tracks
	}

	for i, tr := range allTracks {
		if tr.Number == trackNumber {
			currentTrack = tr
			// Ищем следующий трек
			if i+1 < len(allTracks) && allTracks[i+1].Number == trackNumber+1 {
				nextTrack = allTracks[i+1]
			}
			break
		}
	}

	if currentTrack == nil || nextTrack == nil {
		return 0 // Трек не найден или он последний
	}

	startTime := currentTrack.StartTime().AsDuration()
	nextStartTime := nextTrack.StartTime().AsDuration()

	// Если следующий трек в другом файле, его время начинается с нуля,
	// и мы не можем вычислить общую длительность без знания длительности аудиофайла.
	// Это ограничение формата CUE.
	if currentTrack.parentFile != nextTrack.parentFile {
		return 0
	}

	if nextStartTime < startTime {
		return 0 // Некорректные данные в CUE.
	}

	return nextStartTime - startTime
}

// Cuesheet — это корневая структура, представляющая весь CUE-файл.
// Она содержит глобальные метаданные и список файлов.
type Cuesheet struct {
	Title      string
	Performer  string
	Songwriter string
	Catalog    string   // Media Catalog Number (MCN).
	Files      []*File  // Список файлов, связанных с этим CUE sheet.
	Rem        []string // Список всех комментариев (REM).
	CDTextFile string   // Путь к внешнему файлу CD-TEXT.
}

// NewTimecodeFromFrames создает объект Timecode из общего количества фреймов.
func NewTimecodeFromFrames(totalFrames int) Timecode {
	if totalFrames < 0 {
		totalFrames = 0
	}
	minutes := totalFrames / (60 * FramesPerSecond)
	totalFrames %= (60 * FramesPerSecond)
	seconds := totalFrames / FramesPerSecond
	frames := totalFrames % FramesPerSecond
	return Timecode{Minutes: minutes, Seconds: seconds, Frames: frames}
}
