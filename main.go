package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("pdf-password-remover", flag.ContinueOnError)
	flags.SetOutput(stderr)
	input := flags.String("input", "", "Ruta del PDF protegido")
	password := flags.String("password", "", "Contraseña de apertura o de propietario")
	output := flags.String("output", "", "Ruta del nuevo PDF sin contraseña; no debe existir")
	flags.StringVar(input, "i", "", "Alias de --input")
	flags.StringVar(password, "p", "", "Alias de --password")
	flags.StringVar(output, "o", "", "Alias de --output")
	flags.Usage = func() {
		fmt.Fprintln(stderr, `Quita la contraseña de un PDF y guarda una copia sin cifrado.

Uso:
  pdf-password-remover -i entrada.pdf -p 'clave' -o salida.pdf
  pdf-password-remover --input entrada.pdf --password 'clave' --output salida.pdf

Opciones:
  -i, --input RUTA       Ruta del PDF protegido. Obligatorio.
  -p, --password CLAVE   Contraseña de apertura o de propietario. Obligatorio.
  -o, --output RUTA      Ruta de la copia sin contraseña. Obligatorio.
  -h, --help             Muestra esta ayuda y termina.

Ejemplo:
  pdf-password-remover -i "/ruta/mi archivo.pdf" -p 'mi-contraseña' -o "/ruta/sin-clave.pdf"

Puedes combinar las opciones cortas y largas. Para una clave vacía, usa -p ''.
El original se conserva. La salida no debe existir y su carpeta debe existir.
La contraseña puede quedar en el historial y en la lista de procesos.

Códigos de salida: 0 éxito o ayuda, 1 error de archivo, 2 argumentos inválidos.`)
	}
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	passwordSet := false
	flags.Visit(func(f *flag.Flag) {
		if f.Name == "password" || f.Name == "p" {
			passwordSet = true
		}
	})
	if *input == "" || !passwordSet || *output == "" || flags.NArg() != 0 {
		fmt.Fprintln(stderr, "Error: indica -i/--input, -p/--password y -o/--output, sin argumentos adicionales.")
		flags.Usage()
		return 2
	}
	if err := decrypt(*input, *password, *output); err != nil {
		fmt.Fprintln(stderr, "Error:", err)
		return 1
	}
	fmt.Fprintf(stdout, "PDF sin contraseña guardado en %s\n", *output)
	return 0
}

func decrypt(inputPath, password, outputPath string) (err error) {
	input, err := os.Open(inputPath)
	if err != nil {
		return fmt.Errorf("abrir entrada: %w", err)
	}
	defer input.Close()

	output, err := os.OpenFile(outputPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("crear salida: %w", err)
	}
	defer func() {
		if closeErr := output.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("cerrar salida: %w", closeErr))
		}
		if err != nil {
			err = errors.Join(err, os.Remove(outputPath))
		}
	}()

	conf := model.NewStatelessConfiguration()
	conf.Offline = true
	conf.UserPW = password
	conf.OwnerPW = password
	return api.Decrypt(context.Background(), input, output, conf)
}
