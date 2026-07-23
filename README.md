# Windy NMEA Plugin

Windy plugin plus a small Windows bridge for showing a Trimble GPS NMEA position on the Windy map.

## What It Does

- Windy plugin displays the live GPS marker.
- `windy-nmea-bridge.exe` reads Trimble raw TCP NMEA.
- The bridge exposes a local WebSocket endpoint for the Windy plugin.
- Settings are handled by the bridge at `http://127.0.0.1:8787/settings`.

## Windows Bridge

Run:

```powershell
windy-nmea-bridge.exe
```

The settings page opens automatically. Configure:

- Trimble GPS IP
- Trimble GPS port
- Run automatically when Windows starts
- Start receiving automatically when this program runs

The Windy plugin bridge endpoint is:

```text
127.0.0.1:8787
```

## Windy Plugin Development

Install dependencies:

```powershell
npm install
```

Start the Windy plugin dev server:

```powershell
npm start
```

Open Windy developer mode:

```text
https://www.windy.com/developer-mode
```

Load:

```text
https://localhost:9999/plugin.js
```

Select `GGA` or `RMC` in the plugin, then connect to:

```text
127.0.0.1:8787
```

## Build

Build the Windy plugin on Windows:

```powershell
npm run build:win
```

Build the Windows bridge:

```powershell
cd bridge-native
go build -trimpath -ldflags="-s -w -H windowsgui" -o ..\release\windy-nmea-bridge.exe .
```

## Release

Pushing a tag like `v0.1.0` runs `.github/workflows/release.yml`.
The release uploads:

- `windy-nmea-bridge.exe`
- `windy-plugin-nmea.zip`

Publishing to Windy still uses `.github/workflows/publish-plugin.yml` and requires the GitHub Actions secret `WINDY_API_KEY`.
