package gocue

import (
	"strings"
	"testing"
)

// TestParse_Full is a comprehensive test for a valid, feature-rich CUE sheet.
func TestParse_Full(t *testing.T) {
	cueSheetContent := `
REM This is a comment.
REM Another comment line.
CATALOG 1234567890123
PERFORMER "Various Artists"
TITLE "Ultimate Soundtrack"
SONGWRITER "Main Composer"

FILE "cd1.wav" WAVE
  TRACK 01 AUDIO
    TITLE "First Track"
    PERFORMER "Artist One"
    ISRC US-S1Z-99-00001
    FLAGS DCP PRE
    INDEX 00 00:00:00
    INDEX 01 00:02:30
  TRACK 02 AUDIO
    TITLE "Second Track (with quotes)"
    PERFORMER "Artist Two"
    SONGWRITER "Another Writer"
    PREGAP 00:02:00
    INDEX 01 04:30:15

FILE "cd2.flac" WAVE
  TRACK 03 AUDIO
    TITLE "Third Track"
    PERFORMER "Artist Three"
    INDEX 01 00:00:00
`
	reader := strings.NewReader(cueSheetContent)
	sheet, err := Parse(reader)
	if err != nil {
		t.Fatalf("Parse() returned an unexpected error: %v", err)
	}

	if sheet == nil {
		t.Fatal("Parse() returned a nil sheet")
	}

	// --- Test Global Headers ---
	if sheet.Catalog != "1234567890123" {
		t.Errorf("got Catalog %q, want %q", sheet.Catalog, "1234567890123")
	}
	if sheet.Title != "Ultimate Soundtrack" {
		t.Errorf("got Title %q, want %q", sheet.Title, "Ultimate Soundtrack")
	}
	if sheet.Performer != "Various Artists" {
		t.Errorf("got Performer %q, want %q", sheet.Performer, "Various Artists")
	}
	if sheet.Songwriter != "Main Composer" {
		t.Errorf("got Songwriter %q, want %q", sheet.Songwriter, "Main Composer")
	}
	if len(sheet.Rem) != 2 {
		t.Errorf("got %d REMs, want 2", len(sheet.Rem))
	}

	// --- Test Files ---
	if len(sheet.Files) != 2 {
		t.Fatalf("got %d Files, want 2", len(sheet.Files))
	}
	if sheet.Files[0].Name != "cd1.wav" || sheet.Files[0].Type != "WAVE" {
		t.Errorf("File 1 has wrong name/type: got %s %s", sheet.Files[0].Name, sheet.Files[0].Type)
	}
	if sheet.Files[1].Name != "cd2.flac" || sheet.Files[1].Type != "WAVE" {
		t.Errorf("File 2 has wrong name/type: got %s %s", sheet.Files[1].Name, sheet.Files[1].Type)
	}

	// --- Test Tracks ---
	if len(sheet.Files[0].Tracks) != 2 {
		t.Fatalf("File 1 has %d tracks, want 2", len(sheet.Files[0].Tracks))
	}
	track1 := sheet.Files[0].Tracks[0]
	if track1.Number != 1 || track1.Type != "AUDIO" {
		t.Errorf("Track 1 has wrong number/type: got %d %s", track1.Number, track1.Type)
	}
	if track1.Title != "First Track" {
		t.Errorf("Track 1 title got %q, want %q", track1.Title, "First Track")
	}
	if track1.Performer != "Artist One" {
		t.Errorf("Track 1 performer got %q, want %q", track1.Performer, "Artist One")
	}
	if track1.ISRC != "US-S1Z-99-00001" {
		t.Errorf("Track 1 ISRC got %q, want %q", track1.ISRC, "US-S1Z-99-00001")
	}
	if len(track1.Flags) != 2 || track1.Flags[0] != "DCP" || track1.Flags[1] != "PRE" {
		t.Errorf("Track 1 flags got %v, want [DCP PRE]", track1.Flags)
	}
	if len(track1.Indices) != 2 || track1.Indices[1].Time.String() != "00:02:30" {
		t.Errorf("Track 1 indices not parsed correctly")
	}

	track2 := sheet.Files[0].Tracks[1]
	if track2.Title != "Second Track (with quotes)" {
		t.Errorf("Track 2 title got %q, want %q", track2.Title, "Second Track (with quotes)")
	}
	if track2.Pregap.String() != "00:02:00" {
		t.Errorf("Track 2 pregap got %q, want %q", track2.Pregap.String(), "00:02:00")
	}
	if track2.Songwriter != "Another Writer" {
		t.Errorf("Track 2 songwriter got %q, want %q", track2.Songwriter, "Another Writer")
	}

	// --- Test Duration Calculation ---
	expectedDuration := track2.StartTime().AsDuration() - track1.StartTime().AsDuration()
	if track1.Duration() != expectedDuration {
		t.Errorf("Track 1 duration got %v, want %v", track1.Duration(), expectedDuration)
	}

	if track2.Duration() != 0 {
		t.Errorf("Track 2 duration (before new FILE) got %v, want 0", track2.Duration())
	}

	lastTrack := sheet.Files[1].Tracks[0]
	if lastTrack.Duration() != 0 {
		t.Errorf("Last track duration got %v, want 0", lastTrack.Duration())
	}
}

// TestParse_ErrorCases tests various malformed inputs.
func TestParse_ErrorCases(t *testing.T) {
	testCases := []struct {
		name    string
		input   string
		wantErr string
	}{
		{
			name:    "Command out of context (TRACK before FILE)",
			input:   "TRACK 01 AUDIO",
			wantErr: "line 1: TRACK command found outside of a FILE context",
		},
		// --- ИЗМЕНЕНИЕ: ЭТОТ ТЕСТ УДАЛЕН ---
		// Этот случай больше не является ошибкой. Наш парсер теперь корректно
		// обрабатывает INDEX перед TRACK, чтобы поддерживать файлы,
		// сгенерированные некоторыми программами.
		// {
		// 	name:    "Command out of context (INDEX before TRACK)",
		// 	input:   "FILE \"a.wav\" WAVE\nINDEX 01 00:00:00",
		// 	wantErr: "line 2: INDEX command found outside of a TRACK context",
		// },
		{
			name:    "Incomplete command (FILE)",
			input:   "FILE \"a.wav\"",
			wantErr: "line 1: FILE command requires name and type arguments",
		},
		{
			name:    "Malformed timecode (wrong parts)",
			input:   "FILE \"a.wav\" WAVE\nTRACK 01 AUDIO\nINDEX 01 00:00",
			wantErr: "line 3: invalid timecode for INDEX: timecode must be in MM:SS:FF format",
		},
		{
			name:    "Malformed timecode (bad seconds)",
			input:   "FILE \"a.wav\" WAVE\nTRACK 01 AUDIO\nINDEX 01 00:60:00",
			wantErr: "line 3: invalid timecode for INDEX: seconds value cannot exceed 59: 60",
		},
		{
			name:    "Malformed timecode (bad frames)",
			input:   "FILE \"a.wav\" WAVE\nTRACK 01 AUDIO\nINDEX 01 00:00:75",
			wantErr: "line 3: invalid timecode for INDEX: frames value must be less than 75: 75",
		},
		{
			name:    "Mismatched quotes",
			input:   "TITLE \"My Album",
			wantErr: `line 1: invalid quoting: mismatched quotes`,
		},
		{
			name:    "Invalid track number",
			input:   "FILE \"a.wav\" WAVE\nTRACK aa AUDIO",
			wantErr: `line 2: invalid track number: strconv.Atoi: parsing "aa": invalid syntax`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			reader := strings.NewReader(tc.input)
			_, err := Parse(reader)
			if err == nil {
				t.Fatalf("Parse() did not return an error, but one was expected")
			}
			if err.Error() != tc.wantErr {
				t.Errorf("Parse() returned wrong error.\ngot:  %v\nwant: %v", err, tc.wantErr)
			}
		})
	}
}
