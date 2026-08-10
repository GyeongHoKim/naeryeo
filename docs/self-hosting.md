# 자체 호스팅 (MOTIS)

자체 호스팅 경로 검색 서버를 준비하는 방식에 대해 설명합니다.

## 레포지터리 필요

deploy 폴더 내의 docker compose가 필요합니다.

```bash
git clone https://github.com/GyeongHoKim/naeryeo.git
cd naeryeo
```

## 데이터

다음과 같은 폴더를 생성하고

```bash
mkdir -p deploy/motis/data
```

### 대중교통 시간표 GTFS

**공식 경로 (권장)**: 국가교통DB(KTDB)가 전국 GTFS를 제공합니다.

- 안내 공지: [(안내) 2023년 3월 기준 GTFS 기반정보 제공 안내](https://www.ktdb.go.kr/www/selectBbsNttView.do?key=45&bbsNo=2&nttNo=3764)
- 신청 경로: [국가교통DB](https://www.ktdb.go.kr) → 정보공개 → 자료신청 →
  교통분석자료 신청 → 교통망 GIS DB → 대중교통 → 대중교통

파일명을 **`ktdb-gtfs.zip`으로 바꾸고** `deploy/motis/data/`에 두면 됩니다.

zip 안에 `agency.txt`·`calendar.txt`·`routes.txt`·`stop_times.txt`·`stops.txt`·`trips.txt`가
바로 들어 있어야 합니다.

**미러 (참고)**: 오픈소스 프로젝트 [Transitous](https://github.com/public-transport/transitous/blob/main/feeds/kr.json)가
같은 데이터의 미러를 유지합니다.

### 도로 & 보행 네트워크 OSM

```bash
curl -fL -o deploy/motis/data/south-korea-latest.osm.pbf \
  https://download.geofabrik.de/asia/south-korea-latest.osm.pbf
```

```bash
ls -lh deploy/motis/data/south-korea-latest.osm.pbf
```

출처: [Geofabrik south-korea](https://download.geofabrik.de/asia/south-korea.html)

## 4. 엔진 실행

준비된 파일을 확인합니다.

```
deploy/motis/
├── compose.yaml
├── config.yml
└── data/
    ├── ktdb-gtfs.zip
    └── south-korea-latest.osm.pbf
```

```bash
cd deploy/motis
docker compose --profile import run --rm motis-import
```

**데이터를 갱신할 때만 다시** 실행하면 되고, 서버를 켤 때마다 할 필요는 없습니다.

서버 기동:

```bash
cd deploy/motis
docker compose up -d motis-server
docker compose ps        # 수동으로 다시 실행해 STATUS를 확인합니다
```

## naeryeo와 연결

```bash
naeryeo setup
```

또는,

```bash
naeryeo setup --provider=motis --motis-url=http://localhost:8080 --geocoder=none
```

연결되면 상용 API 키 없이 바로 검색됩니다.

```bash
naeryeo route --from 강남역 --to 홍대입구역
```
