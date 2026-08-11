package investigation

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"reflect"
	"strings"
	"testing"
)

func TestWriteBundlePreservesManifestAndSelectedPattyLog(t *testing.T) {
	input := pattyLog(
		intervalJSON("bundle", 0, logTime0, nil),
		intervalJSON("bundle", 1, logTime1, []string{"GET /checkout HTTP/1.1"}),
		intervalJSON("bundle", 2, logTime2, nil),
	)
	reader := strings.NewReader(input)
	plan, err := PlanSelection(reader, selectionRequest("bundle", logTime1, logTime2))
	if err != nil {
		t.Fatalf("plan bundle selection: %v", err)
	}

	var output bytes.Buffer
	if err := WriteBundle(&output, reader, plan); err != nil {
		t.Fatalf("write bundle: %v", err)
	}
	archive, err := zip.NewReader(bytes.NewReader(output.Bytes()), int64(output.Len()))
	if err != nil {
		t.Fatalf("open bundle: %v", err)
	}
	if got, want := len(archive.File), 2; got != want {
		t.Fatalf("bundle entries = %d, want %d", got, want)
	}
	if archive.File[0].Name != ManifestEntryName || archive.File[1].Name != PattyLogEntryName {
		t.Fatalf("bundle entry order = %q, %q", archive.File[0].Name, archive.File[1].Name)
	}
	for _, entry := range archive.File {
		if entry.Method != zip.Deflate {
			t.Errorf("entry %s compression = %d, want deflate", entry.Name, entry.Method)
		}
		if got := entry.Mode().Perm(); got != 0o600 {
			t.Errorf("entry %s mode = %o, want 600", entry.Name, got)
		}
		if !entry.Modified.Equal(mustTime(logTime2)) {
			t.Errorf("entry %s modified = %s, want selected log-time %s", entry.Name, entry.Modified, logTime2)
		}
	}

	manifestBytes := readZipEntry(t, archive.File[0])
	if !bytes.HasSuffix(manifestBytes, []byte("\n")) || !bytes.Contains(manifestBytes, []byte("\n  \"bundle_schema\"")) {
		t.Fatalf("manifest is not indented JSON with a final newline:\n%s", manifestBytes)
	}
	var manifest Manifest
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		t.Fatalf("decode manifest: %v", err)
	}
	if !reflect.DeepEqual(manifest, plan.Manifest) {
		t.Fatalf("bundle manifest = %#v, want %#v", manifest, plan.Manifest)
	}

	pattyLogBytes := readZipEntry(t, archive.File[1])
	if bytes.Contains(pattyLogBytes, []byte(`"interval":0`)) {
		t.Fatalf("bundle contains unselected interval:\n%s", pattyLogBytes)
	}
	for _, wanted := range []string{`"event_type":"session_start"`, `"interval":1`, `"interval":2`, `GET /checkout`} {
		if !bytes.Contains(pattyLogBytes, []byte(wanted)) {
			t.Errorf("bundle PattyLog does not contain %q", wanted)
		}
	}
}

func TestWriteBundleRejectsNilPlan(t *testing.T) {
	if err := WriteBundle(io.Discard, strings.NewReader(""), nil); err == nil {
		t.Fatal("nil plan was accepted")
	}
}

func readZipEntry(t *testing.T, entry *zip.File) []byte {
	t.Helper()
	reader, err := entry.Open()
	if err != nil {
		t.Fatalf("open ZIP entry %s: %v", entry.Name, err)
	}
	defer reader.Close()
	contents, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("read ZIP entry %s: %v", entry.Name, err)
	}
	return contents
}
