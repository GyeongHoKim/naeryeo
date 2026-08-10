# deploy/motis

## 파일

| 파일 | 용도 |
| --- | --- |
| `compose.yaml` | MOTIS import(그래프 빌드)와 server(HTTP 제공)를 분리한 Docker Compose 구성 |
| `config.yml` | import 설정. `motis config` 생성물에서 타일 블록을 걷어낸 것 — 이유는 파일 주석 참조 |
| `data/` | GTFS zip과 OSM pbf를 두는 자리. **git에 커밋되지 않습니다**(`.gitignore`) |

`data/`에 둘 파일 이름은 `config.yml`이 참조하므로 아래와 같아야 합니다.

```
data/
├── ktdb-gtfs.zip
└── south-korea-latest.osm.pbf
```

## Quick Start

```bash
docker compose --profile import run --rm motis-import
docker compose up -d motis-server
docker compose ps          # (healthy)가 될 때까지 기다립니다
naeryeo setup --provider=motis --motis-url=http://localhost:8080 --geocoder=none
```
