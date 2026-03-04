package checksumutil

import "testing"

func TestSupportedAlgorithms(t *testing.T) {
	got := SupportedAlgorithms()
	if len(got) != 3 || got[0] != "md5" || got[1] != "sha1" || got[2] != "sha256" {
		t.Fatalf("SupportedAlgorithms()=%v", got)
	}
}

func TestComputeHexAndLength(t *testing.T) {
	for _, tc := range []struct {
		algo string
		len  int
	}{
		{algo: "md5", len: 32},
		{algo: "sha1", len: 40},
		{algo: "sha256", len: 64},
	} {
		hex, err := ComputeHex(tc.algo, []byte("hello"))
		if err != nil {
			t.Fatalf("ComputeHex(%s): %v", tc.algo, err)
		}
		if len(hex) != tc.len {
			t.Fatalf("ComputeHex(%s) len=%d, want %d", tc.algo, len(hex), tc.len)
		}
		l, ok := HexLength(tc.algo)
		if !ok || l != tc.len {
			t.Fatalf("HexLength(%s)=(%d,%v), want (%d,true)", tc.algo, l, ok, tc.len)
		}
	}
}

func TestUnsupportedAlgorithm(t *testing.T) {
	if _, err := ComputeHex("crc32", []byte("hello")); err == nil {
		t.Fatalf("expected error for unsupported algorithm")
	}
	if IsSupportedAlgorithm("crc32") {
		t.Fatalf("expected unsupported algorithm")
	}
}
