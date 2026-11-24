# Clientes de Lumina
- `webos/`: LG webOS. Empaquetar: `npx @webos-tools/cli ares-package webos` → `.ipk`; instalar con modo desarrollador (`ares-setup-device`, `ares-install`).
- `android/`: Android, Android TV y Fire TV (ver su README).
La web ya incluye el modo TV (`tv.js`): se activa con `?tv=1` o al detectarse webOS/Android TV/Fire TV.
