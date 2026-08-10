package mediaprocessing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Stable, owner-facing error codes. The media-processing status endpoint
// surfaces these verbatim, so they must never change and must never embed
// file content, paths, or command output.
const (
	// ErrorInvalidVideo is returned for input that ffprobe cannot read or that
	// is structurally invalid: unparseable probe output, no stream at all, no
	// video stream, a subtitle/attachment/data/unknown stream, a malformed or
	// missing duration or frame rate, or missing dimensions.
	ErrorInvalidVideo = "invalid_video"
	// ErrorUnsupportedCodec is returned when the video codec is not H.264 or
	// the (single) audio codec is not in the input allowlist.
	ErrorUnsupportedCodec = "unsupported_codec"
	// ErrorTooLong is returned when the duration exceeds MaxDurationSeconds.
	ErrorTooLong = "too_long"
	// ErrorTooLargeDims is returned when the video exceeds MaxWidth x MaxHeight.
	ErrorTooLargeDims = "too_large_dims"
	// ErrorTooHighFPS is returned when the frame rate exceeds MaxFrameRate.
	ErrorTooHighFPS = "too_high_fps"
	// ErrorTooLarge is returned when the file exceeds the input byte cap.
	ErrorTooLarge = "too_large"
	// ErrorMultiStream is returned when the input carries more than one video
	// stream or more than one audio stream.
	ErrorMultiStream = "multi_stream"
	// ErrorTranscodeFailed is returned when ffmpeg exits non-zero.
	ErrorTranscodeFailed = "transcode_failed"
	// ErrorTimeout is returned when ffprobe or ffmpeg exceeds the caller's
	// context deadline.
	ErrorTimeout = "timeout"
)

// Input acceptance limits. These mirror the F-10 acceptance criteria.
const (
	// MaxDurationSeconds caps how long a submitted video may be.
	MaxDurationSeconds = 30
	// MaxWidth is the maximum accepted video width in pixels.
	MaxWidth = 1280
	// MaxHeight is the maximum accepted video height in pixels.
	MaxHeight = 720
	// MaxFrameRate is the maximum accepted frame rate in frames per second.
	MaxFrameRate = 30.0
)

// allowedVideoCodecs is the input video codec allowlist. Browser MediaRecorder
// emits H.264/AVC for MP4 output and VP8/VP9+Opus for WebM output; Chromium
// and Firefox record WebM by default (Playwright's bundled Chromium cannot
// encode H.264 at all), so VP8/VP9 are first-class inputs. Every allowlisted
// input is transcoded to canonical H.264, so rejecting them would break the
// product's own recordings while adding no security value. AV1/HEVC and other
// codecs stay rejected with unsupported_codec before any transcode work.
var allowedVideoCodecs = map[string]bool{"h264": true, "vp8": true, "vp9": true}

// allowedAudioCodecs is the input audio codec allowlist for the at-most-one
// audio stream. AAC is emitted by MP4 recorders; MP3 and Opus are accepted
// because they are common in practice and trivially transcoded to AAC-LC.
var allowedAudioCodecs = map[string]bool{"aac": true, "mp3": true, "opus": true}

// VideoSpec is the validated, canonicalized description of an accepted input
// video. The transcode step consumes HasAudio; the worker persists the rest.
type VideoSpec struct {
	DurationSeconds float64
	Width           int
	Height          int
	FrameRate       float64
	VideoCodec      string
	HasAudio        bool
	AudioCodec      string
}

// ValidationError carries the stable, owner-facing error code for a rejected
// input. Code is always one of the Error* constants above; Reason is for
// operator logs and is never surfaced to owners.
type ValidationError struct {
	Code   string
	Reason string
}

func (e *ValidationError) Error() string {
	if e.Reason == "" {
		return e.Code
	}
	return e.Code + ": " + e.Reason
}

func validationError(code, format string, args ...any) error {
	return &ValidationError{Code: code, Reason: fmt.Sprintf(format, args...)}
}

// ErrorCode extracts the stable ValidationError code carried by err, or ""
// when err is not a validation rejection. Callers use "" to fall back to a
// generic processing-failure code so infrastructure errors (storage, I/O) are
// never misreported as input rejections.
func ErrorCode(err error) string {
	var ve *ValidationError
	if errors.As(err, &ve) {
		return ve.Code
	}
	return ""
}

// ffprobeOutput mirrors the JSON emitted by
// "ffprobe -v error -print_format json -show_streams -show_format".
type ffprobeOutput struct {
	Streams []ffprobeStream `json:"streams"`
	Format  ffprobeFormat   `json:"format"`
}

type ffprobeStream struct {
	CodecType    string `json:"codec_type"`
	CodecName    string `json:"codec_name"`
	Width        int    `json:"width"`
	Height       int    `json:"height"`
	Duration     string `json:"duration"`
	AvgFrameRate string `json:"avg_frame_rate"`
	RFrameRate   string `json:"r_frame_rate"`
}

type ffprobeFormat struct {
	Duration string `json:"duration"`
}

// Validate probes srcPath with ffprobe and enforces the F-10 input rules:
// exactly one video stream and at most one audio stream, no
// subtitle/attachment/data streams, a duration of at most 30 seconds,
// dimensions of at most 1280x720, a frame rate of at most 30 fps, a codec on
// the input allowlist, and a byte size at most maxBytes (pass 0 to skip the
// size check). It returns a VideoSpec on success and a *ValidationError
// carrying a stable Error* code on rejection.
func Validate(ctx context.Context, srcPath string, maxBytes int64, runner CommandRunner) (*VideoSpec, error) {
	if runner == nil {
		runner = OSCommandRunner{}
	}
	stdout, exitCode, err := runner.Run(ctx, "ffprobe",
		"-v", "error",
		"-print_format", "json",
		"-show_streams",
		"-show_format",
		srcPath,
	)
	if err != nil || exitCode != 0 {
		// Polyglot, truncated, or otherwise unreadable input: ffprobe refuses
		// to describe it. The reason is not surfaced to owners.
		return nil, validationError(ErrorInvalidVideo, "ffprobe rejected the input")
	}
	var out ffprobeOutput
	if err := json.Unmarshal(stdout, &out); err != nil {
		return nil, validationError(ErrorInvalidVideo, "unreadable probe output")
	}
	if len(out.Streams) == 0 {
		return nil, validationError(ErrorInvalidVideo, "no media streams found")
	}

	videoCount, audioCount := 0, 0
	var vStream, aStream *ffprobeStream
	for i := range out.Streams {
		s := &out.Streams[i]
		switch s.CodecType {
		case "video":
			videoCount++
			if videoCount == 1 {
				vStream = s
			}
		case "audio":
			audioCount++
			if audioCount == 1 {
				aStream = s
			}
		default:
			// Subtitle, attachment, data, or unknown stream types are never
			// accepted: they indicate unexpected container content.
			return nil, validationError(ErrorInvalidVideo, "unsupported stream type %q", s.CodecType)
		}
	}
	if videoCount == 0 {
		return nil, validationError(ErrorInvalidVideo, "no video stream")
	}
	if videoCount > 1 {
		return nil, validationError(ErrorMultiStream, "%d video streams", videoCount)
	}
	if audioCount > 1 {
		return nil, validationError(ErrorMultiStream, "%d audio streams", audioCount)
	}

	if maxBytes > 0 {
		info, err := os.Stat(srcPath)
		if err != nil {
			return nil, validationError(ErrorInvalidVideo, "cannot stat input")
		}
		if info.Size() > maxBytes {
			return nil, validationError(ErrorTooLarge, "size %d exceeds %d bytes", info.Size(), maxBytes)
		}
	}

	duration, err := parseDuration(out.Format.Duration)
	if err != nil && vStream != nil {
		duration, err = parseDuration(vStream.Duration)
	}
	if err != nil || duration <= 0 {
		return nil, validationError(ErrorInvalidVideo, "malformed or missing duration")
	}
	if duration > MaxDurationSeconds {
		return nil, validationError(ErrorTooLong, "duration %.1fs exceeds %ds", duration, MaxDurationSeconds)
	}

	if vStream.Width <= 0 || vStream.Height <= 0 {
		return nil, validationError(ErrorInvalidVideo, "missing video dimensions")
	}
	if vStream.Width > MaxWidth || vStream.Height > MaxHeight {
		return nil, validationError(ErrorTooLargeDims, "%dx%d exceeds %dx%d", vStream.Width, vStream.Height, MaxWidth, MaxHeight)
	}

	fps, err := parseFrameRate(vStream.AvgFrameRate)
	if err != nil {
		fps, err = parseFrameRate(vStream.RFrameRate)
	}
	if err != nil || fps <= 0 {
		return nil, validationError(ErrorInvalidVideo, "malformed frame rate")
	}
	if fps > MaxFrameRate {
		return nil, validationError(ErrorTooHighFPS, "%.2f fps exceeds %g", fps, MaxFrameRate)
	}

	videoCodec := vStream.CodecName
	if !allowedVideoCodecs[videoCodec] {
		return nil, validationError(ErrorUnsupportedCodec, "video codec %q is not supported", videoCodec)
	}
	var audioCodec string
	if audioCount == 1 {
		audioCodec = aStream.CodecName
		if !allowedAudioCodecs[audioCodec] {
			return nil, validationError(ErrorUnsupportedCodec, "audio codec %q is not supported", audioCodec)
		}
	}

	return &VideoSpec{
		DurationSeconds: duration,
		Width:           vStream.Width,
		Height:          vStream.Height,
		FrameRate:       fps,
		VideoCodec:      videoCodec,
		HasAudio:        audioCount == 1,
		AudioCodec:      audioCodec,
	}, nil
}

// parseDuration parses a ffprobe duration string ("10.0", "N/A"). Missing or
// malformed durations yield an error, which the caller maps to invalid_video.
func parseDuration(s string) (float64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, errors.New("empty duration")
	}
	d, err := strconv.ParseFloat(s, 64)
	if err != nil || d < 0 {
		return 0, errors.New("invalid duration")
	}
	return d, nil
}

// parseFrameRate parses ffprobe frame rates: a plain float ("30", "60") or a
// rational ("30000/1001"). "0/0" and other unparseable values yield an error.
func parseFrameRate(s string) (float64, error) {
	s = strings.TrimSpace(s)
	if s == "" || s == "0/0" {
		return 0, errors.New("unknown frame rate")
	}
	if !strings.Contains(s, "/") {
		f, err := strconv.ParseFloat(s, 64)
		if err != nil || f <= 0 {
			return 0, errors.New("invalid frame rate")
		}
		return f, nil
	}
	parts := strings.SplitN(s, "/", 2)
	num, err1 := strconv.ParseFloat(parts[0], 64)
	den, err2 := strconv.ParseFloat(parts[1], 64)
	if err1 != nil || err2 != nil || num <= 0 || den <= 0 {
		return 0, errors.New("invalid frame rate")
	}
	return num / den, nil
}
