package media

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func testImage(t *testing.T, mime string) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	var b bytes.Buffer
	var err error
	if mime == "image/png" {
		err = png.Encode(&b, img)
	} else {
		err = jpeg.Encode(&b, img, nil)
	}
	if err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestStoreRoundTripContentAddressingAndOwnership(t *testing.T) {
	store := NewStore(t.TempDir())
	for _, mime := range []string{"image/png", "image/jpeg"} {
		data := testImage(t, mime)
		want := append([]byte(nil), data...)
		part, err := store.Put(mime, data)
		if err != nil {
			t.Fatal(err)
		}
		again, err := store.Put(mime, data)
		if err != nil {
			t.Fatal(err)
		}
		if again.ArtifactKey != part.ArtifactKey || part.Type != "image" || len(part.Data) != 0 {
			t.Fatalf("reference: %#v", part)
		}
		data[0] = 'X'
		got, err := store.Resolve(part)
		if err != nil || !bytes.Equal(got, want) {
			t.Fatalf("roundtrip: %v", err)
		}
		got[0] = 'Y'
		got, err = store.Resolve(part)
		if err != nil || !bytes.Equal(got, want) {
			t.Fatal("read exposes mutable storage")
		}
	}
}

func TestStoreRejectsInvalidImagesAndEscapes(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)
	for _, tc := range []struct {
		mime string
		data []byte
	}{{"text/plain", testImage(t, "image/png")}, {"image/jpeg", testImage(t, "image/png")}, {"image/png", []byte("bad")}, {"image/png", make([]byte, MaxImageBytes+1)}} {
		if _, err := store.Put(tc.mime, tc.data); err == nil {
			t.Fatalf("accepted invalid %s", tc.mime)
		}
	}
	part, err := store.Put("image/png", testImage(t, "image/png"))
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"../outside", "/tmp/outside", "media/../outside", "media/not-a-hash.png"} {
		bad := part
		bad.ArtifactKey = key
		if _, err := store.Resolve(bad); err == nil {
			t.Fatalf("accepted %s", key)
		}
	}
	outside := filepath.Join(t.TempDir(), "outside.png")
	if err := os.WriteFile(outside, testImage(t, "image/png"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(dir, part.ArtifactKey)); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(dir, part.ArtifactKey)); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Resolve(part); err == nil {
		t.Fatal("symlink escaped root")
	}
	if _, err := store.Put(part.MIMEType, testImage(t, "image/png")); err == nil {
		t.Fatal("write followed symlink")
	}
}

func TestStoreMissingCorruptAndWriteFailureAreExplicit(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)
	part, err := store.Put("image/png", testImage(t, "image/png"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, part.ArtifactKey), []byte("corrupt"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Resolve(part); err == nil || !strings.Contains(err.Error(), "image unavailable") {
		t.Fatalf("corrupt: %v", err)
	}
	if err := os.Remove(filepath.Join(dir, part.ArtifactKey)); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Resolve(part); err == nil || !strings.Contains(err.Error(), "image unavailable") {
		t.Fatalf("missing: %v", err)
	}
	if err := os.Remove(filepath.Join(dir, "media")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "media"), []byte("block"), 0600); err != nil {
		t.Fatal(err)
	}
	failed, err := store.Put("image/png", testImage(t, "image/png"))
	if err == nil || failed.ArtifactKey != "" {
		t.Fatalf("dangling reference: %#v %v", failed, err)
	}
}
