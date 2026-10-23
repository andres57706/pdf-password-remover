# PDF password remover

CLI en Go que recibe un PDF y su contraseña y crea una copia sin cifrado.
El ejecutable procesa el archivo localmente y no requiere Go, Python ni qpdf para funcionar.

## Uso

```sh
./bin/pdf-password-remover \
  -i "/ruta/protegido.pdf" \
  -p 'mi-contraseña' \
  -o "/ruta/sin-password.pdf"
```

Con los nombres completos:

```sh
./bin/pdf-password-remover \
  --input "/ruta/protegido.pdf" \
  --password 'mi-contraseña' \
  --output "/ruta/sin-password.pdf"
```

Los tres parámetros son obligatorios. Acepta la contraseña de apertura o la de propietario.
Puedes combinar las formas cortas y largas. Para una contraseña vacía, usa `-p ''`.

La herramienta conserva el original y rechaza una salida que ya exista, incluso si es
la misma ruta de entrada o un enlace. La carpeta de salida debe existir.
Si el procesamiento devuelve un error, elimina la salida incompleta.
Crea la copia con permisos `0600` en macOS y Linux.
Un PDF que ya no tiene cifrado produce un error y no crea una copia.

La contraseña pasada por `-p` o `--password` puede aparecer en el historial de la terminal y en
la lista de procesos. La copia de salida queda sin protección. Reescribir un PDF puede
invalidar sus firmas digitales.

## Compilar

Requiere Go 1.26 o posterior para compilar. Usa [pdfcpu v0.16.3](https://github.com/pdfcpu/pdfcpu/releases/tag/v0.16.3).

```sh
CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o bin/pdf-password-remover .
go test ./...
```

El binario se genera para el sistema y la arquitectura donde compilas.
Ayuda y ejemplos con `./bin/pdf-password-remover -h` o `./bin/pdf-password-remover --help`.

Códigos de salida: `0` éxito o ayuda, `1` error al procesar el archivo, `2` argumentos inválidos.
