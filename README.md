# whatsmeow MQTT Bridge

Eigenständige Go-Bridge zwischen MQTT 5 und WhatsApp über `whatsmeow`. Das Protokoll bleibt mit der früheren `whalibmob`-Bridge kompatibel; der spezifischere Standard-Namespace ist `whatsmeow-mqtt-bridge`. Eine bereits in der offiziellen App registrierte Nummer wird beim ersten Start per QR-Code als verknüpftes Gerät angemeldet. Die Sitzung liegt persistent in SQLite.

## Schnellstart im Devcontainer

Das Repository in VS Code öffnen und **Dev Containers: Reopen in Container** wählen. Die Definition liegt unter `.devcontainer/`; Go 1.25 und die benötigten CLI-Werkzeuge existieren nur im Container. Alternativ:

```sh
cp .env.example .env
docker compose up --build
docker compose logs -f bridge
```

Den QR-Code aus den unveränderten Container-Logs in WhatsApp unter **Verknüpfte Geräte** scannen. QR-Inhalte, Store-Schlüssel und Pairing-Daten werden nie über MQTT publiziert. Das Volume `whatsapp-data` muss erhalten bleiben. `WA_PHONE` wird nicht benötigt.

Die Basisdatei startet ausschließlich die Bridge. Für FHEM `MQTT_URL` in `.env` auf den im FHEM-Netzwerk erreichbaren MQTT-Endpunkt setzen und das lokale Netzwerk wie folgt anbinden.

### Lokales FHEM-Docker-Netzwerk

Die lokale Datei `docker-compose.override.yml` wird von Git ignoriert und von `docker compose` automatisch geladen. Beispiel mit deinem tatsächlichen Netzwerknamen:

```yaml
services:
  bridge:
    networks:
      - fhem

networks:
  fhem:
    external: true
    name: mein_fhem_docker_netzwerk
```

Anschließend genügt:

```sh
docker compose up -d --build
docker compose logs -f bridge
```

Der Hostname in `MQTT_URL`, beispielsweise `mqtt://fhem:1883`, muss innerhalb dieses Netzwerks auf den FHEM-MQTT-Server zeigen.

### Optionaler lokaler Mosquitto

Zum Testen ohne FHEM wird der mitgelieferte Broker explizit als Override ergänzt:

```sh
docker compose -f docker-compose.yml -f docker-compose.mqtt.yml up -d --build
```

Dieses Override setzt `MQTT_URL=mqtt://mqtt:1883` für die Bridge. Die Protokollversion kommt weiterhin aus `.env`; zum Testen von MQTT 5 dort `MQTT_PROTOCOL_VERSION=5` setzen.

## Konfiguration

Alle Konfiguration kommt aus der Umgebung. `MQTT_URL` (`mqtt`, `mqtts`, `ws` oder `wss`) hat Vorrang vor `MQTT_HOST`/`MQTT_PORT`; `MQTT_USERNAME` hat Vorrang vor `MQTT_USER`. `MQTT_PROTOCOL_VERSION` wählt `3` für MQTT 3.1.1 (Default, insbesondere für FHEM) oder `5` für MQTT 5. Der interne Default von `MQTT_BASE_TOPIC` ist `whatsmeow-mqtt-bridge`; die mitgelieferte `.env.example` verwendet für die FHEM-Installation bewusst den kürzeren Namespace `whatsapp`. Für bestehende Installationen kann auch `whalibmob` gesetzt werden. Ein abschließender Slash wird entfernt. Weitere Defaults: `WA_DB_PATH=/data/whatsapp_session.db`, `MEDIA_MAX_SIZE_MB=10`, `LOG_LEVEL=info`, `HEALTH_PORT=3000`. `MQTT_CLIENT_ID` sollte je Instanz eindeutig sein.

`MEDIA_ALLOWED_HOSTS` ist eine kommaseparierte Allowlist exakter, kleingeschriebener Hostnamen oder IP-Adressen. Ohne Einträge sind Mediendownloads gesperrt. Jeder Redirect wird neu geprüft und DNS wird vor dem fest gebundenen Verbindungsaufbau aufgelöst, wodurch Redirect- und DNS-Rebinding-Ausbrüche verhindert werden. Private und Loopback-Ziele funktionieren nur, wenn ihr Host/IP ausdrücklich eingetragen ist. Downloads sind gestreamt, zeitlich begrenzt und hart größenbegrenzt; Bilder benötigen `image/*`, Dokumente `application/*` oder `text/*`. Temporärdateien werden nach Upload oder Fehler entfernt.

## MQTT-Kompatibilitätsvertrag

Alle Publishes und Subscriptions nutzen QoS 1; Status ist retained. Topics beginnen mit dem konfigurierten `MQTT_BASE_TOPIC`. Mit der Beispielkonfiguration liegen Commands unter `whatsapp/cmd/send/text`, `send/image`, `send/document`, `status` und `reconnect`; Events liegen unter `whatsapp/event/status`, `message`, `delivery`, `error` und `log`. Das Command-Envelope bleibt:

```json
{"requestId":"req-123","to":"491701234567","payload":{}}
```

Text nutzt `{"text":"Hallo"}`, Bilder `{"url":"https://example/image.jpg","caption":"optional"}` und Dokumente `{"url":"https://example/file.pdf","title":"optional","fileName":"optional.pdf"}`. Empfänger haben 6–15 Ziffern ohne `+`. JSON wird strikt validiert. Pro Sendebefehl entsteht genau ein Delivery- oder Error-Event. Die Fehlercodes `INVALID_PAYLOAD`, `NOT_READY`, `SEND_FAILED`, `MEDIA_DOWNLOAD_FAILED`, `MEDIA_TOO_LARGE`, `MEDIA_HOST_NOT_ALLOWED`, `SESSION_INVALID` und `WHATSAPP_DISCONNECTED` bleiben reserviert und kompatibel.

Die Statuswerte `starting`, `registered`, `connecting`, `ready`, `disconnected` und `error` bleiben erhalten. LWT ist ein retained `disconnected`-Status. Eingehend werden in Version 1 nur Textnachrichten aus Einzelchats weitergegeben. Eigene Nachrichten, Gruppen, Status/Broadcast, Newsletter, Protokollnachrichten und Medien werden ignoriert. Telefonnummer-JIDs werden als Ziffern ausgegeben; bei nicht auflösbaren LID-Absendern bleibt bewusst die kanonische JID erhalten.

## FHEM MQTT2_DEVICE

Ein vollständiges Beispiel für `MQTT_BASE_TOPIC=whatsapp` liegt unter [`examples/fhem/whatsmeow_mqtt_bridge.cfg`](examples/fhem/whatsmeow_mqtt_bridge.cfg). Die Definition empfängt Status-, Nachrichten-, Delivery- und Fehler-Events, ergänzt den UTF-8-sicheren Setter `sendText` und bindet das Device als Push-Gateway in den FHEM-Befehl `msg` ein.

Direkter Versand:

```text
set whatsmeow_mqtt_bridge sendText 491701234567 Hallo aus FHEM
msg push @whatsmeow_mqtt_bridge:491701234567 Hallo über MSG
```

Für ROOMMATE-Routing wird die Nummer am Bewohner hinterlegt:

```text
attr rr_Beispiel msgContactPush whatsmeow_mqtt_bridge:491701234567
msg push @rr_Beispiel Hallo über ROOMMATE
```

Die Empfängernummer muss 6 bis 15 Ziffern enthalten und ohne führendes `+`, Leerzeichen oder Bindestriche angegeben werden. Der Text darf Leerzeichen und Unicode-Zeichen enthalten. Das Device verwendet das bereits zugeordnete `MQTT2_FHEM_Server` als IODev; bei einem anderen Namen muss das Attribut `IODev` entsprechend angepasst werden.

## Betrieb und Migration

`GET /healthz` meldet Prozess-Liveness. `GET /readyz` liefert nur dann 200, wenn MQTT und WhatsApp angemeldet sind. SIGINT/SIGTERM führen zu geordnetem Shutdown. MQTT und Whatsmeow verbinden transient automatisch neu; ein WhatsApp-Logout setzt den Zustand auf `error` und erfordert neues Pairing. Für ein neues Pairing Container stoppen, das dedizierte Datenvolume bewusst sichern oder entfernen und neu starten. Eine alte `whalibmob`-Sitzung kann nicht migriert werden. Bestehende MQTT-Konsumenten verwenden für eine schrittweise Migration `MQTT_BASE_TOPIC=whalibmob`.

Architekturentscheidung: `modernc.org/sqlite` wird ohne CGO genutzt. Die genaue interne Paketaufteilung ist gegenüber dem Plan leicht verdichtet. Strukturierte Logs bleiben ausschließlich auf stdout/stderr; `event/log` wird absichtlich nicht beschrieben, um Pairing- oder Schlüsselmaterial nicht versehentlich dauerhaft zu verteilen.

## Tests und manueller Smoke-Test

Im Devcontainer:

```sh
go test ./...
go test -race ./...
go vet ./...
```

Smoke-Test: mit frischem dediziertem SQLite-Volume starten, QR scannen, einen Einzelchat-Text empfangen und auf `whatsapp/event/message` prüfen. Danach Text über `whatsapp/cmd/send/text` senden und Delivery prüfen. Container ohne Löschen des Volumes neu starten und automatische Anmeldung kontrollieren. Abschließend Broker und Internet jeweils kurz unterbrechen und Reconnect, retained Status/LWT sowie `/readyz` prüfen. Bild und Dokument von explizit erlaubten Hosts senden und Größenlimit, nicht erlaubten Host und Redirect auf einen nicht erlaubten Host negativ testen.
