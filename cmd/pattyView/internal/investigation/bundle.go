package investigation

import (
	"archive/zip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"
)

const ManifestEntryName = "manifest.json"

// WriteBundle writes a compact incident archive from an established selection
// plan. The selected PattyLog remains streamed from its original source.
func WriteBundle(output io.Writer, input io.ReadSeeker, plan *SelectionPlan) error {
	if plan == nil {
		return errors.New("nil investigation selection plan")
	}
	modified, err := time.Parse(time.RFC3339Nano, plan.Manifest.Range.ThroughLogTime)
	if err != nil {
		return fmt.Errorf("parse bundle log-time: %w", err)
	}

	archive := zip.NewWriter(output)
	manifestWriter, err := archive.CreateHeader(bundleEntryHeader(ManifestEntryName, modified))
	if err != nil {
		return closeBundleAfterError(archive, fmt.Errorf("create manifest entry: %w", err))
	}
	encoder := json.NewEncoder(manifestWriter)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(plan.Manifest); err != nil {
		return closeBundleAfterError(archive, fmt.Errorf("write manifest entry: %w", err))
	}

	pattyLogWriter, err := archive.CreateHeader(bundleEntryHeader(PattyLogEntryName, modified))
	if err != nil {
		return closeBundleAfterError(archive, fmt.Errorf("create PattyLog entry: %w", err))
	}
	if err := plan.WritePattyLog(input, pattyLogWriter); err != nil {
		return closeBundleAfterError(archive, err)
	}
	if err := archive.Close(); err != nil {
		return fmt.Errorf("finish incident bundle: %w", err)
	}
	return nil
}

func bundleEntryHeader(name string, modified time.Time) *zip.FileHeader {
	header := &zip.FileHeader{
		Name:     name,
		Method:   zip.Deflate,
		Modified: modified,
	}
	header.SetMode(0o600)
	return header
}

func closeBundleAfterError(archive *zip.Writer, original error) error {
	if closeErr := archive.Close(); closeErr != nil {
		return fmt.Errorf("%w; closing incomplete bundle: %v", original, closeErr)
	}
	return original
}
