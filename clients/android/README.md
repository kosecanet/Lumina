# Lumina para Android / Android TV / Fire TV

WebView a pantalla completa que carga tu servidor Lumina. Sin dependencias externas.

## Compilar el APK
- **Android Studio:** abre esta carpeta → Build → Build APK(s). Resultado: `app/build/outputs/apk/debug/app-debug.apk`.
- **Línea de comandos:** `./gradlew assembleDebug` (JDK 17 + Android SDK 34).
- **Sin instalar nada:** sube esta carpeta a un repositorio de GitHub; la acción `.github/workflows/android.yml` genera el APK como artefacto.

## Instalar
- **Fire TV:** activa *Opciones de desarrollador → Apps de orígenes desconocidos / Depuración ADB* y `adb connect IP_FIRETV:5555 && adb install app-debug.apk`
  (o la app *Downloader* con un enlace al APK).
- **Android TV / móvil:** `adb install app-debug.apk`.

## Uso
Al abrir pide la dirección del servidor (`192.168.1.50:8780`). Se guarda. **Tecla amarilla o Menú**: cambiar de servidor.
En TV se activa el modo mando automáticamente (flechas, OK, Atrás).
