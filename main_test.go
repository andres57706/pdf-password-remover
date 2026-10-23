package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

func readFile(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func encryptedPDF(t *testing.T, aes bool, bits int, userPassword string) []byte {
	t.Helper()
	conf := model.NewStatelessConfiguration()
	conf.EncryptUsingAES = aes
	conf.EncryptKeyLength = bits
	conf.UserPW = userPassword
	conf.OwnerPW = "owner-secret"
	var encrypted bytes.Buffer
	if err := api.Encrypt(t.Context(), bytes.NewReader(readFile(t, "testdata/plain.pdf")), &encrypted, conf); err != nil {
		t.Fatal(err)
	}
	return encrypted.Bytes()
}

func TestDecrypt(t *testing.T) {
	for _, algorithm := range []struct {
		name     string
		aes      bool
		bits     int
		password string
	}{
		{"rc4-40", false, 40, "user-secret"},
		{"rc4-128", false, 128, "user-secret"},
		{"aes-128", true, 128, "user-secret"},
		{"aes-256", true, 256, "user-secret"},
		{"aes-256-empty", true, 256, ""},
		{"aes-256-unicode", true, 256, "contraseña con espacios"},
	} {
		t.Run(algorithm.name, func(t *testing.T) {
			encrypted := encryptedPDF(t, algorithm.aes, algorithm.bits, algorithm.password)
			for _, credential := range []struct{ name, password string }{
				{"user", algorithm.password},
				{"owner", "owner-secret"},
			} {
				t.Run(credential.name, func(t *testing.T) {
					dir := t.TempDir()
					input := filepath.Join(dir, "protected file.pdf")
					output := filepath.Join(dir, "plain file.pdf")
					if err := os.WriteFile(input, encrypted, 0o600); err != nil {
						t.Fatal(err)
					}
					var stdout, stderr bytes.Buffer
					if code := run([]string{"--input", input, "--password", credential.password, "--output", output}, &stdout, &stderr); code != 0 {
						t.Fatalf("exit %d: %s", code, &stderr)
					}
					ctx, err := api.ReadAndValidate(t.Context(), bytes.NewReader(readFile(t, output)), model.NewStatelessConfiguration())
					if err != nil {
						t.Fatal(err)
					}
					if ctx.Encrypt != nil || ctx.PageCount != 1 {
						t.Fatalf("unexpected PDF: encrypted=%t, pages=%d", ctx.Encrypt != nil, ctx.PageCount)
					}
					content, err := pdfcpu.ExtractPageContent(t.Context(), ctx, 1)
					if err != nil {
						t.Fatal(err)
					}
					text, err := io.ReadAll(content)
					if err != nil || !bytes.Contains(text, []byte("(Password remover test)")) {
						t.Fatalf("page content lost: %q, %v", text, err)
					}
					if !bytes.Equal(readFile(t, input), encrypted) {
						t.Fatal("input was modified")
					}
					info, err := os.Stat(output)
					if err != nil {
						t.Fatal(err)
					}
					if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
						t.Fatalf("output permissions: %o", info.Mode().Perm())
					}
					if stdout.Len() == 0 || stderr.Len() != 0 {
						t.Fatalf("unexpected messages: stdout=%q stderr=%q", &stdout, &stderr)
					}
				})
			}
		})
	}
}

func TestFlagAliases(t *testing.T) {
	encrypted := encryptedPDF(t, true, 256, "")
	for _, test := range []struct{ name, input, password, output string }{
		{"short", "-i", "-p", "-o"},
		{"long", "--input", "--password", "--output"},
		{"mixed", "-i", "--password", "-o"},
	} {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			input := filepath.Join(dir, "protected.pdf")
			output := filepath.Join(dir, "plain.pdf")
			if err := os.WriteFile(input, encrypted, 0o600); err != nil {
				t.Fatal(err)
			}
			var stderr bytes.Buffer
			if code := run([]string{test.input, input, test.password, "", test.output, output}, io.Discard, &stderr); code != 0 {
				t.Fatalf("exit %d: %s", code, &stderr)
			}
			ctx, err := api.ReadAndValidate(t.Context(), bytes.NewReader(readFile(t, output)), model.NewStatelessConfiguration())
			if err != nil {
				t.Fatal(err)
			}
			if ctx.Encrypt != nil {
				t.Fatal("output is still encrypted")
			}
		})
	}
}

func TestFailureRemovesOutput(t *testing.T) {
	for _, test := range []struct {
		name  string
		input []byte
	}{
		{"wrong-password", encryptedPDF(t, true, 256, "user-secret")},
		{"invalid-pdf", []byte("not a PDF")},
		{"unencrypted-pdf", readFile(t, "testdata/plain.pdf")},
		{"missing-input", nil},
	} {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			input := filepath.Join(dir, "input.pdf")
			output := filepath.Join(dir, "output.pdf")
			if test.input != nil {
				if err := os.WriteFile(input, test.input, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			var stdout, stderr bytes.Buffer
			if code := run([]string{"-input", input, "-password", "wrong-secret", "-output", output}, &stdout, &stderr); code != 1 {
				t.Fatalf("exit %d, want 1: %s", code, &stderr)
			}
			if _, err := os.Lstat(output); !os.IsNotExist(err) {
				t.Fatalf("output left behind: %v", err)
			}
			if test.input != nil && !bytes.Equal(readFile(t, input), test.input) {
				t.Fatal("input was modified")
			}
			if stdout.Len() != 0 || stderr.Len() == 0 || bytes.Contains(stderr.Bytes(), []byte("wrong-secret")) {
				t.Fatalf("unexpected messages: stdout=%q stderr=%q", &stdout, &stderr)
			}
		})
	}
}

func TestRefusesExistingOutput(t *testing.T) {
	encrypted := encryptedPDF(t, true, 256, "user-secret")
	for _, name := range []string{"existing", "same-input", "symlink", "hardlink"} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			input := filepath.Join(dir, "input.pdf")
			output := filepath.Join(dir, "output.pdf")
			if err := os.WriteFile(input, encrypted, 0o600); err != nil {
				t.Fatal(err)
			}
			var err error
			switch name {
			case "existing":
				err = os.WriteFile(output, encrypted, 0o600)
			case "same-input":
				output = input
			case "symlink":
				err = os.Symlink(input, output)
			case "hardlink":
				err = os.Link(input, output)
			}
			if err != nil {
				t.Fatal(err)
			}
			if code := run([]string{"-input", input, "-password", "user-secret", "-output", output}, io.Discard, io.Discard); code != 1 {
				t.Fatalf("exit %d, want 1", code)
			}
			if !bytes.Equal(readFile(t, input), encrypted) || !bytes.Equal(readFile(t, output), encrypted) {
				t.Fatal("existing file was modified")
			}
		})
	}
}

func TestArguments(t *testing.T) {
	for _, test := range []struct {
		name string
		args []string
		code int
	}{
		{"short-help", []string{"-h"}, 0},
		{"long-help", []string{"--help"}, 0},
		{"no-arguments", nil, 2},
		{"unknown-flag", []string{"-unknown"}, 2},
		{"missing-input", []string{"-password", "test", "-output", "out.pdf"}, 2},
		{"missing-password", []string{"-input", "in.pdf", "-output", "out.pdf"}, 2},
		{"missing-output", []string{"-input", "in.pdf", "-password", "test"}, 2},
		{"missing-value", []string{"-password"}, 2},
		{"extra-argument", []string{"-input", "in.pdf", "-password", "test", "-output", "out.pdf", "extra"}, 2},
	} {
		t.Run(test.name, func(t *testing.T) {
			var stderr bytes.Buffer
			if code := run(test.args, io.Discard, &stderr); code != test.code {
				t.Fatalf("exit %d, want %d: %s", code, test.code, &stderr)
			}
			if stderr.Len() == 0 {
				t.Fatal("missing help or error message")
			}
			if test.code == 0 {
				for _, expected := range []string{"Uso:", "-i, --input", "-p, --password", "-o, --output", "-h, --help", "Ejemplo:"} {
					if !bytes.Contains(stderr.Bytes(), []byte(expected)) {
						t.Errorf("help is missing %q", expected)
					}
				}
			}
		})
	}
}
