# Lumina

Servidor multimedia ligero (un solo binario Go, sin dependencias) con interfaz web estilo "video bajo demanda":
bibliotecas de vídeo locales, series, continuar viendo, y TV en directo por Xtream con guía (EPG).

## Ejecutar
    ./lumina                          # http://servidor:8780, datos en ./data
    ./lumina -addr :8780 -data /ruta/datos -password MICLAVE

- `-password` (o variable `LUMINA_PASSWORD`): activa login. **Sin contraseña cualquiera en tu red puede entrar.**
- `ffmpeg` y `ffprobe` son opcionales (junto al ejecutable o en el PATH): dan miniaturas, duración, y
  reproducción de formatos no compatibles con el navegador (remux/transcodificación solo bajo demanda).
- Primer uso: Ajustes → Bibliotecas → añadir carpeta del servidor. Ajustes → TV en directo → pegar URL M3U o datos Xtream, elegir canales.

## Reproducción
Directo (Range) → remux a MP4 (copia de vídeo, audio a AAC) → x264 solo si el navegador falla.
En modo remux no hay subtítulos externos.

## TV en directo y conexiones
Lumina abre **una sola conexión** al proveedor por canal y la reparte entre todos los dispositivos que ven ese canal.
Si ya hay otro canal en uso y tu proveedor solo permite 1 conexión, se avisa y puedes forzar el cambio.
No evita los límites del proveedor.

## Compilar
    ./build.sh     # genera dist/ para linux amd64/arm64 y windows amd64
Requiere Go 1.24+. Prueba con proveedor simulado: `go run ./tools/mockxtream` (demo/demo, puerto 9080).

## Pendiente
Clientes nativos con ExoPlayer (reproducción 4K/HDR, códecs extra).

## Clientes
Ver `clients/`: LG webOS (`.ipk`) y Android / Android TV / Fire TV (proyecto Gradle). El modo TV de la web usa el mando.

## Televisores y dispositivos antiguos
El servidor detecta LG webOS y cualquier Chromium < 90 (p. ej. Fire TV antiguos) y les sirve `web/legacy/`, una versión rebajada
(JS para Chrome 53+, polyfills, CSS sin `gap`/`aspect-ratio`/`inset`). Se regenera con `node tools/legacy/build.mjs <ruta a esbuild>`
(`npm i esbuild`) cada vez que cambies `web/`. Forzarla: añade `?legacy=1`.

## Actualizaciones desde GitHub
Repositorio oficial: `kosecanet/Lumina` (cámbialo en Ajustes → Sistema). Para publicar una versión:

    git tag v0.3.0 && git push origin v0.3.0

La acción `release.yml` compila los binarios (Linux amd64/arm64, Windows), el `.ipk` de LG, el APK de Android/Fire TV y `checksums.txt`,
y crea la release. Lumina comprueba cada 6 h; en Ajustes → Sistema aparece «Instalar vX». El binario se verifica con SHA-256,
se comprueba que arranca y se reinicia solo (conserva `lumina.old` por si hay que volver atrás).
Instalar desde la web exige haber arrancado con `-password`. Opcional: «instalar automáticamente» cuando nadie esté viendo nada.
Repos privados: variable `LUMINA_GH_TOKEN`. Los clientes de TV/Android cargan la web del servidor, así que se actualizan con él.
Para ejecutarlo como servicio: `deploy/lumina.service`.

## Cuentas y permisos

Por defecto Lumina no pide usuario (o usa una contraseña global con `-password`). En **Ajustes → Usuarios → Activar cuentas** creas el
administrador y, desde ese momento, cada persona entra con su usuario y contraseña:

- Cada cuenta tiene su propio historial («seguir viendo»), visto/no visto y canales favoritos.
- Permisos por usuario: qué bibliotecas ve y si puede usar la TV en directo. Solo los administradores gestionan bibliotecas, TV, usuarios, actualizaciones y ajustes.
- Las contraseñas se guardan con PBKDF2-SHA256 y la sesión va en una cookie firmada. Hay límite de intentos por IP.
- Si olvidas la contraseña de la cuenta administradora, arranca con `-password CLAVE` (o `LUMINA_PASSWORD`) y entra con el usuario `admin` y esa clave: entra como el primer administrador.
- Los permisos limitan lo que ve cada cuenta, no el cupo del proveedor: con una sola conexión Xtream, el directo se comparte igual.
