# Migration von SQLite zu PostgreSQL

Diese Anleitung übernimmt die bestehende WhatsApp-Sitzung einschließlich Geräteidentität,
Schlüsseln, Sessions und App-State. Ein erneutes Pairing per QR-Code ist danach normalerweise
nicht erforderlich.

> **Wichtig:** SQLite und PostgreSQL dürfen während der Migration nicht gleichzeitig von
> Bridge-Instanzen mit derselben WhatsApp-Sitzung verwendet werden. Die SQLite-Datei enthält
> geheimes Schlüsselmaterial und muss wie ein Passwort behandelt werden.

## Voraussetzungen

- Die Bridge läuft vor und nach der Migration mit derselben Version.
- `sqlite3`, `psql` und [pgloader](https://pgloader.readthedocs.io/en/latest/ref/sqlite.html)
  sind auf dem Migrationshost installiert.
- Die PostgreSQL-Datenbank beziehungsweise das Schema wurde wie im
  [PostgreSQL-Abschnitt der README](../README.md#postgresql) angelegt.
- Die PostgreSQL-Zieldatenbank enthält noch keine produktive WhatsApp-Sitzung.

Die Beispiele verwenden:

```sh
SQLITE_DB=/pfad/zur/whatsapp_session.db
read -rsp 'PostgreSQL-DSN: ' PG_DSN
printf '\n'
read -rsp 'PostgreSQL-Admin-DSN für den Import: ' PG_MIGRATION_DSN
printf '\n'
```

Als DSN beispielsweise `postgresql://whatsmeow_bridge:URL_ENCODED_PASSWORD@postgres:5432/whatsmeow_bridge`
eingeben. Das Passwort muss URL-kodiert sein. Den DSN nicht in die Shell-History, Logs oder das
Repository schreiben. `PG_MIGRATION_DSN` zeigt auf dieselbe Datenbank, verwendet aber einen
PostgreSQL-Administrator; pgloader benötigt ihn kurzzeitig zum Deaktivieren der Fremdschlüssel.

## 1. SQLite aktualisieren und Bridge stoppen

Die alte Bridge einmal mit der zu migrierenden Version und weiterhin mit
`WA_DB_DRIVER=sqlite` starten. Dadurch bringt whatsmeow die SQLite-Datenbank auf die zu dieser
Version passende Schema-Version. Danach die Bridge vollständig stoppen:

```sh
docker compose stop bridge
```

Prüfen, dass kein anderer Container oder Prozess dieselbe SQLite-Datei verwendet. Ab jetzt
bis zum abgeschlossenen Import darf die alte Bridge nicht wieder gestartet werden.

## 2. SQLite sichern und prüfen

Bei einem Docker-Volume zuerst den tatsächlichen Pfad oder eine Kopie der Datei aus dem Volume
ermitteln. Anschließend ein konsistentes Backup erzeugen:

```sh
sqlite3 "$SQLITE_DB" ".backup '${SQLITE_DB}.pre-postgres-migration'"
chmod 600 "${SQLITE_DB}.pre-postgres-migration"
sqlite3 "$SQLITE_DB" "PRAGMA integrity_check;"
```

`PRAGMA integrity_check` muss `ok` ausgeben. Die Anzahl der registrierten Geräte notieren:

```sh
sqlite3 "$SQLITE_DB" \
  "SELECT version FROM whatsmeow_version; SELECT count(*) FROM whatsmeow_device;"
```

Für eine reguläre Bridge-Installation wird genau ein Eintrag in `whatsmeow_device` erwartet.

## 3. Leeres PostgreSQL-Schema initialisieren

Die Bridge mit der neuen PostgreSQL-Konfiguration starten, aber **keinen QR-Code scannen**.
Der Start legt über die whatsmeow-Migrationen alle Tabellen mit den korrekten
PostgreSQL-Datentypen an:

```sh
docker compose \
  -f docker-compose.yml \
  -f docker-compose.postgres.yml \
  up -d postgres bridge
docker compose stop bridge
```

Bei einer externen PostgreSQL-Instanz die dafür verwendeten Compose-Dateien und
`WA_DB_*`-Werte entsprechend anpassen.

Vor dem Import muss das Ziel leer sein:

```sh
psql "$PG_DSN" -v ON_ERROR_STOP=1 -c \
  "SELECT count(*) AS devices FROM whatsmeow_device;"
```

Der Wert muss `0` sein. Ist er größer, nicht fortfahren: Ziel prüfen oder eine neue, leere
Datenbank beziehungsweise ein neues Schema verwenden.

### Eigenes Schema in einer gemeinsam genutzten Datenbank

Bei `WA_DB_SCHEMA=whatsmeow` muss pgloader ebenfalls dieses Schema als `search_path` verwenden.
Das kann ein PostgreSQL-Administrator für die Dauer der Migration datenbankspezifisch setzen:

```sql
ALTER ROLE whatsmeow_bridge IN DATABASE fhem SET search_path TO whatsmeow;
ALTER ROLE postgres IN DATABASE fhem SET search_path TO whatsmeow;
```

Die zweite Anweisung an den tatsächlichen Administrator aus `PG_MIGRATION_DSN` anpassen.
Danach neu verbinden und mit `SHOW search_path;` für beide DSNs kontrollieren. Der temporäre
`search_path` des Administrators kann nach der Migration mit
`ALTER ROLE postgres IN DATABASE fhem RESET search_path;` zurückgesetzt werden.

## 4. Daten mit pgloader übertragen

pgloader wird bewusst mit `data only` verwendet. Das von whatsmeow angelegte Zielschema bleibt
dadurch unverändert:

```sh
pgloader \
  --with "data only" \
  --with "disable triggers" \
  "sqlite://$SQLITE_DB" \
  "$PG_MIGRATION_DSN"
```

Der Lauf muss ohne `ERROR` enden. Nicht einfach erneut in dasselbe Ziel importieren, da die
Primärschlüssel dann bereits vorhanden sind. Nach einem fehlgeschlagenen Lauf eine neue leere
Datenbank beziehungsweise ein neues leeres Schema anlegen und den Import wiederholen.

pgloader aktiviert die zuvor deaktivierten Trigger am Ende wieder. Ein Fehler beim
Reaktivieren ist daher ebenfalls ein fehlgeschlagener Import und darf nicht ignoriert werden.

## 5. Import verifizieren

Schema-Version und Gerätezahl müssen den zuvor notierten SQLite-Werten entsprechen:

```sh
psql "$PG_DSN" -v ON_ERROR_STOP=1 -c \
  "SELECT version FROM whatsmeow_version; SELECT count(*) FROM whatsmeow_device;"
```

Optional alle whatsmeow-Tabellen auflisten und ihre Zeilenzahlen prüfen:

```sh
psql "$PG_DSN" -v ON_ERROR_STOP=1 -c \
  "SELECT schemaname, relname, n_live_tup
     FROM pg_stat_user_tables
    WHERE relname LIKE 'whatsmeow_%'
    ORDER BY relname;"
```

`n_live_tup` ist eine Statistik und kann unmittelbar nach dem Import noch ungenau sein. Für
kritische Tabellen kann stattdessen ein exaktes `SELECT count(*)` in beiden Datenbanken
ausgeführt werden.

## 6. Auf PostgreSQL umschalten

Die Bridge ausschließlich mit der PostgreSQL-Konfiguration starten:

```sh
docker compose \
  -f docker-compose.yml \
  -f docker-compose.postgres.yml \
  up -d bridge
docker compose logs -f bridge
```

Erwartet werden eine vorhandene Registrierung und anschließend der Status `ready`, ohne dass
ein neuer QR-Code gescannt werden muss. Danach eine eingehende und eine ausgehende
Textnachricht testen sowie `/readyz` prüfen.

Die SQLite-Datei noch nicht löschen. Sie dient bis zum erfolgreichen Funktionstest als
Rollback-Kopie und sollte offline sowie mit restriktiven Dateirechten aufbewahrt werden.

## Rollback

1. PostgreSQL-Bridge stoppen.
2. PostgreSQL-Konfiguration entfernen beziehungsweise `WA_DB_DRIVER=sqlite` setzen.
3. Falls nötig `${SQLITE_DB}.pre-postgres-migration` als SQLite-Datenbank wiederherstellen.
4. Genau eine Bridge mit SQLite starten.

Nach dem ersten erfolgreichen WhatsApp-Betrieb auf PostgreSQL ist die alte SQLite-Kopie nicht
mehr aktuell. Ein späterer Wechsel zurück auf diesen Stand kann deshalb Nachrichten- und
