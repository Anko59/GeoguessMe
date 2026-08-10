package mediaprocessing

import (
	"context"
	"errors"
)

// Transcode re-encodes srcPath into a canonical MP4/H.264 object at dstPath.
// Output is always H.264 with the yuv420p pixel format, fast-start layout,
// and AAC-LC audio when the input has an audio stream; metadata, chapters,
// subtitles, and data streams are stripped. The per-job wall-clock bound is
// the caller's context deadline (the worker uses 60s); the child additionally
// inherits the package resource limits.
//
// The returned error carries ErrorTranscodeFailed when ffmpeg exits non-zero
// and ErrorTimeout when the context deadline expires. dstPath is left in a
// partial state on failure; the caller is responsible for deleting it through
// the durable cleanup path.
func Transcode(ctx context.Context, srcPath, dstPath string, hasAudio bool, runner CommandRunner) error {
	if runner == nil {
		runner = OSCommandRunner{}
	}
	_, exitCode, err := runner.Run(ctx, "ffmpeg", transcodeArgs(srcPath, dstPath, hasAudio)...)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return validationError(ErrorTimeout, "transcode exceeded its deadline")
		}
		return validationError(ErrorTranscodeFailed, "transcode did not complete")
	}
	if exitCode != 0 {
		return validationError(ErrorTranscodeFailed, "transcode exited with status %d", exitCode)
	}
	return nil
}

// transcodeArgs builds the canonical ffmpeg argument vector. Only the first
// video stream and (optionally) the first audio stream are mapped; chapters,
// metadata, subtitles, and data streams are stripped. The -fflags/-flags
// +bitexact pair and the explicit "-metadata title=" keep output bytes
// deterministic and metadata-free, and +faststart places the moov atom
// first so the file streams while still downloading. The legacy faststart
// flag name is required: the pinned alpine ffmpeg 8.1.2-r0 build rejects the
// newer fast_start spelling (and +fast-start) with a movflags parse error.
func transcodeArgs(srcPath, dstPath string, hasAudio bool) []string {
	args := []string{
		"-nostdin",
		"-y",
		"-i", srcPath,
		"-map", "0:v:0",
	}
	if hasAudio {
		args = append(args, "-map", "0:a:0")
	}
	args = append(args,
		"-c:v", "libx264",
		"-pix_fmt", "yuv420p",
		"-preset", "fast",
		"-movflags", "+faststart",
		"-metadata", "title=",
		"-map_metadata", "-1",
		"-map_chapters", "-1",
		"-sn",
		"-dn",
		"-fflags", "+bitexact",
		"-flags:v", "+bitexact",
		"-flags:a", "+bitexact",
	)
	if hasAudio {
		args = append(args, "-c:a", "aac", "-b:a", "128k")
	}
	return append(args, dstPath)
}
