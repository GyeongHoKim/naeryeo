# deploy/motis

naeryeo가 경로 검색에 사용할 수 있는 **자체 호스팅 MOTIS 인스턴스**를 띄우는 파일들입니다.

이 디렉터리의 파일만 보고 따라 하지 마세요. 데이터 준비(무엇을 어디서 받는지), 자원 요구치,
데이터의 한계, naeryeo와의 연결 방법은 모두 아래 문서에 있습니다.

➡️ **[docs/self-hosting.md](../../docs/self-hosting.md)**

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

## 요약

```bash
# 1. data/ 에 위 두 파일 배치 (docs/self-hosting.md 3절 참조)
# 2. 그래프 빌드 — 한 번만. 실측 55초 / 최대 3.98 GiB / 결과 1.5 GB
docker compose --profile import run --rm motis-import
# 3. 서버 기동
docker compose up -d motis-server
docker compose ps          # (healthy)가 될 때까지 기다립니다
# 4. naeryeo 연결
naeryeo setup --provider=motis --motis-url=http://localhost:8080 --geocoder=none
```
