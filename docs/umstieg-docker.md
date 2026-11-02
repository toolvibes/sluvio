# Umstieg: bestehendes HAProxy-Compose-Setup um Sluvio erweitern

Diese Anleitung geht von einem typischen Setup aus: HAProxy läuft mit dem offiziellen Image, die Config ist als einzelne Datei read-only gemountet:

```yaml
services:
  haproxy:
    image: haproxy:2.9-alpine
    container_name: my-haproxy
    volumes:
      - ./haproxy.cfg:/usr/local/etc/haproxy/haproxy.cfg:ro
    ports: ["80:80", "8081:8081", "8404:8404"]
```

An der Config selbst musst du nichts ändern. Sluvio liest sie so, wie sie ist, auch mit Kommentaren und auskommentierten Services.

## 1. Config in ein eigenes Verzeichnis verschieben

```bash
cd deployments/haproxy
mkdir haproxy
git mv haproxy.cfg haproxy/haproxy.cfg   # oder: mv haproxy.cfg haproxy/
sudo chown -R 99:99 haproxy               # UID 99 = Benutzer "haproxy" im Image
```

HAProxy braucht nur Lesezugriff, Sluvio schreibt. Willst du die Datei weiterhin selbst bearbeiten, geht das mit `sudo`, oder du gibst deiner Gruppe Schreibrechte (`sudo chmod -R g+w haproxy` und eine passende Gruppe).

## 2. Compose-Datei anpassen

```yaml
services:
  haproxy:
    image: haproxy:2.9-alpine
    container_name: my-haproxy
    restart: unless-stopped
    # neu: Master-Socket für Status, Wartung ohne Reload und Reloads
    command: ["haproxy", "-W", "-db", "-f", "/usr/local/etc/haproxy/haproxy.cfg", "-S", "/var/lib/haproxy/master.sock"]
    volumes:
      - ./haproxy:/usr/local/etc/haproxy          # neu: Verzeichnis statt Datei, ohne :ro
      - haproxy-run:/var/lib/haproxy               # neu
    ports: ["80:80", "8081:8081", "8404:8404"]
    healthcheck:
      test: ["CMD", "haproxy", "-c", "-f", "/usr/local/etc/haproxy/haproxy.cfg"]
      interval: 30s
      timeout: 5s
      retries: 3

  sluvio:                                       # neu
    image: ghcr.io/toolvibes/sluvio:haproxy2.9
    container_name: sluvio
    restart: unless-stopped
    depends_on: [haproxy]
    environment:
      SLUVIO_MASTER_SOCKET: /var/lib/haproxy/master.sock
      SLUVIO_ADMIN_PASSWORD: ${SLUVIO_ADMIN_PASSWORD:?in .env setzen}
    volumes:
      - ./haproxy:/usr/local/etc/haproxy
      - haproxy-run:/var/lib/haproxy
      - sluvio-data:/var/lib/sluvio
    ports:
      - "192.168.1.10:8484:8484"                # nur im LAN erreichbar (eigene IP eintragen)

volumes:
  haproxy-run:
  sluvio-data:
```

In die `.env` neben der Compose-Datei:

```bash
SLUVIO_ADMIN_PASSWORD=ein-langes-passwort
```

## 3. Starten

```bash
docker compose up -d
docker compose logs -f sluvio
```

HAProxy wird dabei einmal neu gestartet, weil sich `command` und Volumes ändern. Danach laufen alle Reloads über Sluvio unterbrechungsfrei.

Öffne `http://<host>:8484`, melde dich als `admin` an und prüfe unter **Einrichtung**, dass alle Punkte grün sind.

## 4. Optional: Zugriff über deinen Reverse Proxy

Wenn du Sluvio über deinen vorgelagerten Reverse Proxy (z. B. Nginx Proxy Manager) erreichbar machen willst, leg einen eigenen Host an, zum Beispiel `haproxy-admin.example.com` → `http://192.168.1.10:8484`, und schränke ihn per Access-List auf dein LAN bzw. VPN ein. Sluvio sollte nicht offen im Internet stehen.

## Was Sluvio an deiner Config ändert und was nicht

- Ohne dein Zutun ändert Sluvio nichts, auch nicht beim ersten Start. Es formatiert nichts um und legt keine Datenbank an.
- Jede Änderung zeigt dir vorher den Diff. Nur die betroffenen Zeilen ändern sich.
- **Deaktivieren** bedeutet Auskommentieren, so wie du es bisher von Hand machst. Vorbereitete, auskommentierte Services (ACLs, `use_backend` und `backend`-Block) erscheinen in Sluvio als deaktivierte Services und lassen sich per Schalter aktivieren.
- Jede Version liegt im Verlauf (Volume `sluvio-data`) und lässt sich wiederherstellen.
