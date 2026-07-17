# Implementierungsplan: whatsmeow MQTT Bridge

## Ziel

Eine eigenstaendige Go-Anwendung ersetzt intern `whalibmob` durch
[`go.mau.fi/whatsmeow`](https://github.com/tulir/whatsmeow), bleibt aber auf der
MQTT-Seite kompatibel zur bestehenden `whalibmob-mqtt-bridge`.

Die WhatsApp-Nummer wird nicht durch die Bridge registriert. Sie muss bereits in
der offiziellen WhatsApp-App funktionieren. Beim ersten Start wird die Bridge per
QR-Code als verknuepftes Geraet angemeldet; danach wird die Sitzung persistent in
SQLite gespeichert.

## Verbindlicher Kompatibilitaetsvertrag

Der historische Standardwert bleibt absichtlich:

```text
MQTT_BASE_TOPIC=whalibmob
```

### Command Topics

```text
whalibmob/cmd/send/text
whalibmob/cmd/send/image
whalibmob/cmd/send/document
whalibmob/cmd/status
whalibmob/cmd/reconnect
```

Alle Commands erwarten weiterhin dieses Envelope:

```json
{
  "requestId": "req-123",
  "to": "491701234567",
  "payload": {}
}
```

Payloads:

- Text: `{"text":"Hallo"}`
- Bild: `{"url":"https://example/image.jpg","caption":"optional"}`
- Dokument: `{"url":"https://example/file.pdf","title":"optional","fileName":"optional.pdf"}`
- Status und Reconnect akzeptieren aus Kompatibilitaetsgruenden ebenfalls das
  bisherige Envelope, auch wenn `to` und `payload` semantisch nicht benoetigt werden.

### Event Topics

```text
whalibmob/event/status
whalibmob/event/message
whalibmob/event/delivery
whalibmob/event/error
whalibmob/event/log
```

Alle Publish- und Subscribe-Operationen verwenden QoS 1. Statusmeldungen werden
retained publiziert.

Status:

```json
{
  "state": "ready",
  "connected": true,
  "phone": "491701234567",
  "message": "optional"
}
```

Bestehende States bleiben erhalten:

```text
starting, registered, connecting, ready, disconnected, error
```

Neue Pairing-Zustaende duerfen spaeter additiv hinzukommen, duerfen bestehende
Konsumenten aber nicht voraussetzen.

Erfolgreiches Senden (`event/delivery`):

```json
{
  "requestId": "req-123",
  "status": "ok",
  "messageId": "3EB0...",
  "timestamp": "2026-07-17T12:34:56.000Z"
}
```

Fehler (`event/error`):

```json
{
  "requestId": "req-123",
  "status": "error",
  "errorCode": "NOT_READY",
  "message": "WhatsApp client is not ready",
  "timestamp": "2026-07-17T12:34:56.000Z"
}
```

Zu erhaltende Fehlercodes:

```text
INVALID_PAYLOAD
NOT_READY
SEND_FAILED
MEDIA_DOWNLOAD_FAILED
MEDIA_TOO_LARGE
MEDIA_HOST_NOT_ALLOWED
SESSION_INVALID
WHATSAPP_DISCONNECTED
```

Eingehende Nachricht (`event/message`):

```json
{
  "messageId": "3EB0...",
  "from": "491701234567",
  "type": "text",
  "payload": {"text":"Hallo"},
  "timestamp": "2026-07-17T12:34:56.000Z"
}
```

## Technische Leitentscheidungen

- Go mit einer im `go.mod` fixierten Go-Version.
- WhatsApp: `go.mau.fi/whatsmeow`.
- Sitzung: `go.mau.fi/whatsmeow/store/sqlstore` mit SQLite.
- Bevorzugter SQLite-Treiber: `modernc.org/sqlite`, sofern er mit der aktuellen
  sqlstore-API problemlos funktioniert; andernfalls `github.com/mattn/go-sqlite3`.
- MQTT 5: `github.com/eclipse/paho.golang/autopaho` und
  `github.com/eclipse/paho.golang/paho`.
- Terminal-QR: `github.com/mdp/qrterminal/v3`.
- Strukturierte Logs nach stdout/stderr; keine Geheimnisse oder QR-Inhalte in
  dauerhafte MQTT-Logs publizieren.
- Sauberes Shutdown ueber `context.Context`, SIGINT und SIGTERM.
- Keine globale mutable Business-Logik; externe Clients hinter kleinen Interfaces,
  damit Unit-Tests ohne WhatsApp und MQTT moeglich sind.

## Vorgesehene Projektstruktur

```text
cmd/bridge/main.go
internal/config/
internal/bridge/
internal/mqttbridge/
internal/whatsapp/
internal/media/
internal/model/
internal/health/
Dockerfile
docker-compose.yml
docker-compose.fhem-network.yml
.env.example
README.md
```

Die genaue Paketaufteilung darf beim Implementieren vereinfacht werden, solange
Abhaengigkeiten und Testbarkeit sauber bleiben.

## Implementierungsetappen

### 1. Grundgeruest und Konfiguration

- Go-Modul initialisieren.
- Konfiguration ausschliesslich aus Environment laden und validieren.
- Unterstuetzte Variablen:
  - `MQTT_URL`, alternativ `MQTT_HOST` und `MQTT_PORT`
  - `MQTT_USERNAME`, alternativ `MQTT_USER`
  - `MQTT_PASSWORD`
  - `MQTT_BASE_TOPIC`, Default `whalibmob`
  - `WA_DB_PATH`, Default `/data/whatsapp_session.db`
  - `MEDIA_ALLOWED_HOSTS`
  - `MEDIA_MAX_SIZE_MB`, Default `10`
  - `LOG_LEVEL`, Default `info`
  - `HEALTH_PORT`, Default `3000`
- `WA_PHONE` ist fuer QR-Pairing nicht verpflichtend. Nach erfolgreichem Pairing
  wird die eigene Nummer aus dem whatsmeow Store bezogen.
- Unit-Tests fuer Defaults, Validierung und Topic-Normalisierung schreiben.

### 2. MQTT-Schicht

- `autopaho` mit Auto-Reconnect konfigurieren.
- Eindeutige, konfigurierbare Client-ID verwenden.
- LWT auf `<base>/event/status` als kompatibles Status-JSON mit
  `state=disconnected`, `connected=false`, QoS 1 und retained setzen.
- Bei jeder neuen MQTT-Verbindung alle Command Topics erneut abonnieren.
- Publizieren und Subscription-Callbacks von Business-Logik entkoppeln.
- Payload-Groessen sinnvoll begrenzen.
- Unit-Tests fuer Topic-Mapping, JSON und Routing schreiben.

### 3. whatsmeow Store und Pairing

- SQL-Container oeffnen und Migrationen ausfuehren.
- `GetFirstDevice()` verwenden; genau eine WhatsApp-Sitzung pro Bridge-Instanz.
- Ohne gespeicherte Device-ID vor `Connect()` einen QR-Channel mit abbrechbarem
  Context anlegen.
- Jedes `code`-Event als gut scanbaren QR-Code im Terminal ausgeben.
- `success`, `timeout`, `err-client-outdated` und Pairing-Fehler sauber behandeln.
- QR-Context vor Disconnect oder erneutem Pairing immer canceln.
- Mit vorhandener Device-ID ohne QR verbinden.
- `EnableAutoReconnect` verwenden und Logout strikt von transientem Disconnect
  unterscheiden.
- Niemals Store, Keys, QR-Code oder Pairing-Daten ueber MQTT ausgeben.

### 4. WhatsApp Events nach MQTT

- `events.Message` verarbeiten.
- Zunaechst nur Einzelchat-Textnachrichten abbilden.
- `IsFromMe`, Status-Broadcasts, Newsletter, Gruppen und Protokollnachrichten in
  Version 1 bewusst ignorieren und debug-loggen.
- Text aus normaler Conversation sowie ExtendedTextMessage extrahieren.
- Whatsmeow-Zeitstempel nach UTC RFC3339 mit Millisekunden serialisieren.
- Absender-JIDs robust normalisieren; LID-JIDs nicht blind als Telefonnummer
  ausgeben. Falls keine Telefonnummer aufloesbar ist, die kanonische JID erhalten
  und dies dokumentieren.
- Nachrichten-ID unveraendert zur Deduplizierung publizieren.
- Handler darf nicht dauerhaft blockieren; Backpressure und geordnetes Shutdown
  beruecksichtigen.

### 5. MQTT Commands nach WhatsApp

- JSON strikt validieren und kompatible Fehler publizieren.
- Telefonnummern normalisieren: 6 bis 15 Ziffern, kein fuehrendes `+`, danach
  `types.NewJID(number, types.DefaultUserServer)` verwenden.
- Text mit `SendMessage()` senden.
- Whatsmeow Message-ID und Zeitstempel als Delivery Event zurueckgeben.
- `cmd/status` publiziert den aktuellen Status erneut.
- `cmd/reconnect` trennt kontrolliert und verbindet wieder; ein ausgeloggtes
  Device darf dabei nicht als funktionierende Session ausgegeben werden.
- Gleichzeitige Sendebefehle begrenzen; pro Command genau ein Delivery- oder
  Error-Ergebnis erzeugen.

### 6. Medien

- HTTP/HTTPS-Download fuer Bild und Dokument kompatibel implementieren.
- Nur Hosts aus `MEDIA_ALLOWED_HOSTS` erlauben.
- DNS-Rebinding und Redirects auf nicht erlaubte Hosts verhindern.
- Private/Loopback-Ziele nur erlauben, wenn sie explizit konfiguriert sind.
- Streaming mit hartem Groessenlimit; keine vollstaendige unlimitierte Antwort in
  den Speicher laden.
- MIME-Type pruefen:
  - Bilder: `image/*`
  - Dokumente: `application/*` oder `text/*`
- Temporaere Dateien auch bei Fehlern entfernen.
- Upload ueber whatsmeow und Versand als ImageMessage bzw. DocumentMessage.
- SSRF-, Redirect-, Groessenlimit- und Cleanup-Tests schreiben.

### 7. Status, Health und Betrieb

- Statusaenderungen retained publizieren.
- `GET /healthz`: Prozess lebt.
- `GET /readyz`: nur 200, wenn MQTT und WhatsApp betriebsbereit sind.
- Docker-Multi-Stage-Build als kleiner Non-Root-Container.
- Persistentes Volume fuer `/data`.
- Compose-Beispiele fuer Standard-, Host- und bestehendes FHEM-Netzwerk.
- `.env.example` und Betriebsanleitung inklusive QR-Scan aus Container-Logs.

### 8. Verifikation

- `gofmt`, `go vet ./...` und `go test ./...` muessen erfolgreich sein.
- Race-Test fuer nebenlaeufige Kernpakete: `go test -race ./...`.
- MQTT-Integrationstest gegen Mosquitto, wenn Docker verfuegbar ist.
- WhatsApp wird in automatischen Tests ueber ein Interface gefakt; kein echtes
  Konto und kein QR-Code in CI.
- Manueller Smoke-Test dokumentieren:
  1. frische SQLite-Datei,
  2. QR-Code scannen,
  3. eingehenden Text auf altem Event Topic pruefen,
  4. Text ueber altes Command Topic senden,
  5. Container neu starten und automatische Wiederanmeldung pruefen,
  6. MQTT- und Internet-Unterbrechungen pruefen.

## Definition of Done fuer Version 1

- Erstes Pairing per QR-Code funktioniert und ueberlebt einen Container-Neustart.
- Ein- und ausgehende Einzelchat-Texte funktionieren.
- Bild- und Dokumentversand ueber URL funktionieren mit den bestehenden Payloads.
- Alle bisherigen Topics, Envelopes, Fehlercodes und Kern-Statuswerte sind
  kompatibel.
- MQTT und WhatsApp verbinden sich nach transienten Ausfaellen automatisch neu.
- Logout fuehrt nachvollziehbar in einen nicht-bereiten Zustand und erfordert
  erneutes Pairing.
- Status/LWT, Healthchecks, Docker und persistentes Volume funktionieren.
- README enthaelt Setup, Migration, Sicherheitsgrenzen und Smoke-Test.
- Tests, Vet und Formatierung sind gruen.

## Nicht Bestandteil von Version 1

- Registrierung einer neuen Telefonnummer.
- Gruppen, Channels, Statusmeldungen, Anrufe oder Broadcast-Listen.
- Empfang oder Weiterleitung eingehender Medien.
- Web-UI fuer Pairing.
- Mehrere WhatsApp-Konten in einer Instanz.
- Offizielle WhatsApp Business Cloud API.

## Arbeitsauftrag fuer den Folge-Agenten

Implementiere diesen Plan iterativ im Repository. Beginne mit einem kurzen Audit
des Plans und halte begruendete Abweichungen in `README.md` oder einer
Entscheidungsnotiz fest. Bewahre den MQTT-Kompatibilitaetsvertrag. Fuehre nach jeder
Etappe die passenden Tests aus und hinterlasse keine Platzhalter fuer
sicherheitskritische Pfade wie Medien-Download, Session-Persistenz oder Reconnect.

