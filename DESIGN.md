# Camera Object Recognition Search Demo - Architecture Design

## 1. 概要

本プロジェクトは、ブラウザのカメラ映像からユーザーが指定した枠内のオブジェクトを画像認識し、その認識結果をもとに Web 検索を行うデモアプリケーションである。

MVP では、ユーザーがボタンを押したタイミングでのみ画像認識を実行する。  
自動認識は将来拡張として扱う。

フロントエンドは GitHub Pages にデプロイし、バックエンドは Google Cloud Run 上で Go 製 API として稼働させる。  
LLM ベンダーは後から差し替え可能な構成とし、MVP 時点では Amazon Bedrock に固定する。

- Amazon Bedrock（MVP 実装対象）

Google Cloud Vertex AI などの追加 Provider は、MVP 後の拡張として扱う。

Web 検索 API についても将来的な差し替えを考慮し、検索結果を次のプロジェクトでも再利用しやすい共通データ形式で扱う。

---

## 2. ゴール

### 2.1 MVP のゴール

- GitHub Pages 上で動作する Web アプリを作成する
- ブラウザからカメラを起動できる
- 画面中央などに認識対象の枠を表示できる
- ユーザーがボタンを押すと、枠内画像を切り出せる
- 切り出した画像を Cloud Run API に送信できる
- Cloud Run API が LLM に画像認識を依頼できる
- LLM が以下を返せる
  - オブジェクト名
  - 説明
  - 検索クエリ
  - 確信度相当の情報
- 生成された検索クエリを使って Web 検索できる
- 検索結果を共通スキーマで返却できる
- 検索結果を LLM で短く要約できる
- フロントエンドで認識結果・検索結果・要約を表示できる
- LLM は Amazon Bedrock を使う
- Provider Interface を維持し、MVP 後に LLM ベンダーを追加できる

### 2.2 MVP ではやらないこと

- 自動認識
- リアルタイム AR 表示
- ユーザー認証
- 検索履歴の永続保存
- 大量トラフィック向けの高度なスケーリング設計
- 独自画像認識モデルの学習
- RAG 用ベクトルデータベース連携
- 商品データベースとの照合
- モバイルアプリ化

---

## 3. 全体アーキテクチャ

```text
User Browser
  |
  | GitHub Pages
  v
Frontend: React + TypeScript + Vite
  |
  | 1. Camera起動
  | 2. 枠内画像をcanvasでcrop
  | 3. JPEG/WebPに圧縮
  | 4. APIへ送信
  v
Cloud Run API: Go
  |
  | 5. LLM Provider Interface
  |      └─ Bedrock Adapter (MVP)
  |
  | 6. Web Search Provider Interface
  |      └─ Tavily Adapter (MVP)
  |
  | 7. Search Result Normalizer
  |
  | 8. Optional Summary Generation
  v
Frontend
  |
  | 9. 認識結果・検索結果・要約を表示
  v
User
```

---

## 4. 技術スタック

## 4.1 Frontend

| 項目 | 技術 |
|---|---|
| Framework | React |
| Language | TypeScript |
| Build Tool | Vite |
| Hosting | GitHub Pages |
| Camera API | MediaDevices API |
| Image Processing | Canvas API |
| Styling | CSS Modules / Tailwind CSS / plain CSS のいずれか |

MVP では、実装の見通しが良く GitHub Pages と相性の良い Vite + React + TypeScript を採用する。

## 4.2 Backend

| 項目 | 技術 |
|---|---|
| Runtime | Go |
| Hosting | Google Cloud Run |
| Container | Docker |
| API Style | REST |
| Secret Management | Google Secret Manager または Cloud Run 環境変数 |
| Logging | Cloud Logging |
| Deployment | GitHub Actions |

Go は学習目的も兼ねて採用する。Cloud Run では標準的な HTTP サーバーとして実装できるため、Go の `net/http` を基本とする。

必要に応じて以下の軽量ルーターを検討する。

- `net/http`
- `chi`
- `gin`

MVP では依存を増やしすぎないため、`net/http` または `chi` を推奨する。

## 4.3 LLM

MVP では Amazon Bedrock に固定する。

| Provider | 用途 |
|---|---|
| Amazon Bedrock | Claude / Nova などによる画像認識・検索要約 |

アプリケーション本体は特定ベンダーの SDK やレスポンス形式に依存しない。  
バックエンド内に LLM インターフェースを定義し、Provider ごとの Adapter で差分を吸収する。Vertex AI などの追加 Provider は、同じ Interface に Adapter を追加して対応する。

## 4.4 Web Search

MVP では Tavily に固定する。

| Provider | 特徴 |
|---|---|
| Tavily | LLM アプリ向けに扱いやすい |

検索 API も LLM と同様に Provider Interface を定義し、将来的な差し替えに備える。Brave Search API や Serper は MVP 後の追加 Provider として扱う。

---

## 5. ユースケース

## 5.1 基本ユースケース

```text
1. ユーザーが Web アプリを開く
2. ブラウザがカメラ利用許可を求める
3. ユーザーがカメラ利用を許可する
4. カメラ映像が画面に表示される
5. 画面中央に認識対象の枠が表示される
6. ユーザーがオブジェクトを枠内に入れる
7. ユーザーが「この枠内を検索」ボタンを押す
8. フロントエンドが枠内画像を切り出す
9. フロントエンドが画像を Cloud Run API に送信する
10. バックエンドが LLM に画像認識を依頼する
11. LLM がオブジェクト名・説明・検索クエリを返す
12. バックエンドが検索 API に検索クエリを送る
13. 検索 API が検索結果を返す
14. バックエンドが検索結果を共通スキーマに正規化する
15. バックエンドが必要に応じて LLM に検索結果の要約を依頼する
16. バックエンドがフロントエンドへ結果を返す
17. フロントエンドが認識結果・検索結果・要約を表示する
```

---

## 6. リポジトリ構成

モノレポ構成を推奨する。

```text
.
├── README.md
├── docs/
│   ├── architecture.md
│   ├── api.md
│   ├── deployment.md
│   └── env.md
├── frontend/
│   ├── package.json
│   ├── vite.config.ts
│   ├── tsconfig.json
│   ├── index.html
│   ├── public/
│   └── src/
│       ├── main.tsx
│       ├── App.tsx
│       ├── components/
│       │   ├── CameraView.tsx
│       │   ├── CaptureOverlay.tsx
│       │   ├── ResultPanel.tsx
│       │   ├── SearchResultList.tsx
│       │   └── ProviderBadge.tsx
│       ├── hooks/
│       │   ├── useCamera.ts
│       │   └── useRecognizeSearch.ts
│       ├── lib/
│       │   ├── apiClient.ts
│       │   ├── cropImage.ts
│       │   └── imageCompression.ts
│       ├── types/
│       │   └── api.ts
│       └── styles/
│           └── global.css
├── backend/
│   ├── go.mod
│   ├── go.sum
│   ├── Dockerfile
│   ├── cmd/
│   │   └── server/
│   │       └── main.go
│   ├── internal/
│   │   ├── config/
│   │   │   └── config.go
│   │   ├── handler/
│   │   │   ├── health.go
│   │   │   └── recognize_search.go
│   │   ├── usecase/
│   │   │   └── recognize_search.go
│   │   ├── llm/
│   │   │   ├── llm.go
│   │   │   ├── prompt.go
│   │   │   └── bedrock/
│   │   │       └── client.go
│   │   ├── search/
│   │   │   ├── search.go
│   │   │   └── tavily/
│   │   │       └── client.go
│   │   ├── normalizer/
│   │   │   └── search_result.go
│   │   ├── model/
│   │   │   ├── api.go
│   │   │   ├── llm.go
│   │   │   └── search.go
│   │   └── middleware/
│   │       ├── cors.go
│   │       ├── logging.go
│   │       └── ratelimit.go
├── .github/
│   └── workflows/
│       ├── deploy-frontend.yml
│       └── deploy-backend.yml
└── .gitignore
```

---

## 7. フロントエンド設計

## 7.1 主要コンポーネント

### `CameraView`

責務:

- カメラ起動
- video 要素への stream 接続
- カメラ停止
- カメラ権限エラーの表示

### `CaptureOverlay`

責務:

- 認識対象の枠を表示
- 枠の座標・サイズを管理
- 将来的に枠サイズ変更に対応可能にする

### `ResultPanel`

責務:

- 認識されたオブジェクト名の表示
- 説明文の表示
- 生成された検索クエリの表示
- LLM 要約の表示

### `SearchResultList`

責務:

- 検索結果一覧の表示
- title / url / snippet / source などの表示
- 次プロジェクトで再利用しやすい検索結果データの確認

## 7.2 画像切り出し

フロントエンドでは `canvas` を使い、video 映像のうち認識枠に対応する領域だけを切り出す。

推奨仕様:

```text
出力形式: image/jpeg または image/webp
最大幅: 512px〜1024px
JPEG quality: 0.7〜0.85
```

MVP ではコスト削減のため、画像全体ではなく枠内だけを送信する。

---

## 8. バックエンド設計

## 8.1 レイヤー構成

```text
handler
  ↓
usecase
  ↓
llm interface
  └─ bedrock implementation (MVP)

search interface
  └─ tavily implementation (MVP)

normalizer
  ↓
response
```

## 8.2 Handler

HTTP リクエストの受け取りとレスポンス返却のみを担当する。

主な責務:

- JSON decode
- 入力バリデーション
- context timeout 設定
- usecase 呼び出し
- エラーレスポンス整形

## 8.3 Usecase

ビジネスフローを制御する。

```text
1. 画像を検証する
2. LLM で画像認識する
3. 検索クエリを取得する
4. Web 検索する
5. 検索結果を正規化する
6. LLM で検索結果を要約する
7. API レスポンスを組み立てる
```

## 8.4 LLM Interface

アプリケーション本体は Amazon Bedrock の詳細を知らない。将来的に Vertex AI などを追加しても、Usecase は同じ Interface に依存する。

```go
type VisionLLM interface {
    RecognizeObject(ctx context.Context, req RecognizeObjectRequest) (*RecognizeObjectResponse, error)
    SummarizeSearchResults(ctx context.Context, req SummarizeSearchResultsRequest) (*SummarizeSearchResultsResponse, error)
}
```

## 8.5 Search Interface

検索 API も同様に差し替え可能にする。

```go
type WebSearcher interface {
    Search(ctx context.Context, req SearchRequest) (*SearchResponse, error)
}
```

---

## 9. API 設計

## 9.1 `GET /healthz`

ヘルスチェック用 API。

### Response

```json
{
  "status": "ok"
}
```

---

## 9.2 `POST /api/recognize-search`

枠内画像を受け取り、画像認識・Web 検索・要約を実行する。

### Request

```json
{
  "imageBase64": "data:image/jpeg;base64,...",
  "language": "ja",
  "options": {
    "maxSearchResults": 5
  }
}
```

### Request fields

| Field | Required | Description |
|---|---:|---|
| `imageBase64` | yes | base64 encoded image |
| `language` | no | `ja` or `en`。未指定時は `ja` |
| `options.maxSearchResults` | no | 検索結果数。MVP では 1〜5 の範囲に制限 |

MVP では Provider をリクエスト単位で指定できない。LLM は Bedrock、検索は Tavily に固定する。

### Response

```json
{
  "requestId": "req_01H...",
  "recognizedObject": {
    "objectName": "ワイヤレスイヤホン",
    "description": "画像内の物体はワイヤレスイヤホンの充電ケースのように見えます。",
    "searchQuery": "ワイヤレスイヤホン 充電ケース 特徴 使い方",
    "confidence": "medium",
    "needsMoreContext": false
  },
  "search": {
    "provider": "tavily",
    "query": "ワイヤレスイヤホン 充電ケース 特徴 使い方",
    "results": [
      {
        "id": "sr_001",
        "title": "ワイヤレスイヤホンの選び方",
        "url": "https://example.com/articles/wireless-earbuds",
        "displayUrl": "example.com/articles/wireless-earbuds",
        "snippet": "ワイヤレスイヤホンの特徴や選び方を解説します。",
        "source": "example.com",
        "publishedAt": null,
        "language": "ja",
        "rank": 1,
        "score": 0.92,
        "contentType": "web_page",
        "provider": "tavily",
        "raw": {}
      }
    ]
  },
  "summary": {
    "text": "画像内の物体はワイヤレスイヤホンの充電ケースである可能性があります。検索結果によると、充電ケースはイヤホン本体の保管と充電に使われます。",
    "llmProvider": "bedrock",
    "model": "anthropic.claude-3-5-sonnet-20241022"
  },
  "meta": {
    "llmProvider": "bedrock",
    "searchProvider": "tavily",
    "elapsedMs": 2530
  }
}
```

---

## 10. 共通データモデル

## 10.1 RecognizedObject

```json
{
  "objectName": "string",
  "description": "string",
  "searchQuery": "string",
  "confidence": "low | medium | high",
  "needsMoreContext": false
}
```

### Fields

| Field | Description |
|---|---|
| `objectName` | 認識したオブジェクト名 |
| `description` | 画像から判断できる説明 |
| `searchQuery` | Web 検索に使うクエリ |
| `confidence` | LLM が推定した確信度 |
| `needsMoreContext` | 追加情報が必要かどうか |

---

## 10.2 NormalizedSearchResult

検索結果は次のプロジェクトに繋げやすいよう、Provider 固有形式ではなく共通スキーマで保持する。

```json
{
  "id": "sr_001",
  "title": "string",
  "url": "https://example.com/page",
  "displayUrl": "example.com/page",
  "snippet": "string",
  "source": "example.com",
  "publishedAt": "2026-05-12T00:00:00Z",
  "language": "ja",
  "rank": 1,
  "score": 0.92,
  "contentType": "web_page",
  "provider": "tavily",
  "raw": {}
}
```

### Fields

| Field | Description |
|---|---|
| `id` | アプリ内で付与する検索結果 ID |
| `title` | ページタイトル |
| `url` | 正規 URL |
| `displayUrl` | 表示用 URL |
| `snippet` | 検索結果の短い説明 |
| `source` | ドメインまたは媒体名 |
| `publishedAt` | 公開日。取得できない場合は `null` |
| `language` | 推定言語 |
| `rank` | 検索順位 |
| `score` | Provider から得られる関連度。ない場合は rank から擬似計算 |
| `contentType` | `web_page`, `news`, `video`, `image`, `pdf`, `unknown` など |
| `provider` | 検索 Provider 名 |
| `raw` | Provider 固有レスポンス。再利用・デバッグ用 |

## 10.3 次プロジェクト接続用 Search Artifact

次のプロジェクトで検索結果を RAG、ブックマーク、調査ログ、ナレッジベース化などに繋げやすくするため、API 内部では以下のような Search Artifact として扱う。

```json
{
  "artifactVersion": "1.0",
  "query": {
    "text": "ワイヤレスイヤホン 充電ケース 特徴 使い方",
    "language": "ja",
    "generatedFrom": {
      "type": "image_object_recognition",
      "objectName": "ワイヤレスイヤホン",
      "description": "画像内の物体はワイヤレスイヤホンの充電ケースのように見えます。"
    }
  },
  "results": [
    {
      "id": "sr_001",
      "title": "ワイヤレスイヤホンの選び方",
      "url": "https://example.com/articles/wireless-earbuds",
      "displayUrl": "example.com/articles/wireless-earbuds",
      "snippet": "ワイヤレスイヤホンの特徴や選び方を解説します。",
      "source": "example.com",
      "publishedAt": null,
      "language": "ja",
      "rank": 1,
      "score": 0.92,
      "contentType": "web_page",
      "provider": "tavily",
      "raw": {}
    }
  ],
  "summary": {
    "text": "検索結果の要約テキスト",
    "llmProvider": "bedrock",
    "model": "anthropic.claude-3-5-sonnet-20241022"
  },
  "createdAt": "2026-05-12T00:00:00Z"
}
```

MVP では DB 保存は行わないが、将来的にこの形式をそのまま保存・エクスポートできるようにする。

---

## 11. LLM Provider 固定・拡張設計

## 11.1 環境変数

MVP では Bedrock 固定とする。

```env
LLM_PROVIDER=bedrock
AWS_REGION=us-east-1
BEDROCK_MODEL_ID=anthropic.claude-3-5-sonnet-20241022
```

## 11.2 Provider 選択

MVP ではリクエスト単位の Provider 切り替えは許可しない。

```text
LLM Provider: bedrock fixed
Selection source: backend configuration only
```

将来的に Vertex AI などを追加する場合も、API request で任意の Provider を指定させるのではなく、サーバー側設定で有効化した Provider のみ選択可能にする。

## 11.3 LLM Adapter の責務

各 Adapter は以下を担当する。

- Provider 固有 SDK / HTTP API の呼び出し
- 画像データの Provider 向け形式への変換
- Prompt の構築
- JSON 出力のパース
- エラーの共通形式への変換
- モデル名・利用 Provider の metadata 付与

## 11.4 LLM 共通出力制約

画像認識では、LLM に必ず JSON を返させる。

```json
{
  "objectName": "string",
  "description": "string",
  "searchQuery": "string",
  "confidence": "low | medium | high",
  "needsMoreContext": false
}
```

自然文だけの出力は不可とする。

---

## 12. Search Provider 固定・拡張設計

## 12.1 環境変数

MVP では Tavily 固定とする。

```env
SEARCH_PROVIDER=tavily
TAVILY_API_KEY=xxx
```

## 12.2 Search Adapter の責務

各 Adapter は以下を担当する。

- Provider 固有 API の呼び出し
- Provider 固有レスポンスの取得
- 共通 `NormalizedSearchResult` への変換
- 元レスポンスを `raw` に保存
- rank / score / source などの補完

## 12.3 検索結果の再利用性

次プロジェクトで検索結果を使いやすくするため、以下の方針を採用する。

- URL を必ず保持する
- title / snippet を必ず保持する
- provider 固有レスポンスは `raw` に残す
- rank を必ず保持する
- 検索クエリと生成元情報を保持する
- Artifact の version を持つ
- 将来的な DB 保存を前提に JSON serializable な構造にする

---

## 13. Prompt 設計

## 13.1 画像認識 Prompt

目的:

- 枠内画像の主対象物を推定する
- 検索しやすいクエリを作る
- 不確かな場合は正直に低 confidence を返す

出力形式:

```json
{
  "objectName": "string",
  "description": "string",
  "searchQuery": "string",
  "confidence": "low | medium | high",
  "needsMoreContext": false
}
```

方針:

- 商品名を断定しすぎない
- ブランドや型番は見えている場合のみ含める
- 検索クエリは短く具体的にする
- 日本語 UI の場合は日本語検索クエリにする

## 13.2 検索結果要約 Prompt

入力:

- objectName
- description
- searchQuery
- normalized search results

出力:

```json
{
  "text": "string"
}
```

方針:

- 検索結果にない情報を過剰に補完しない
- 出典 URL に基づく説明にする
- デモ UI では短く表示できるようにする
- key points や caveats の構造化は MVP 後の拡張とする

---

## 14. セキュリティ設計

## 14.1 ブラウザに置かないもの

以下は GitHub Pages 側に置かない。

- AWS access key
- Bedrock credential
- Tavily API key
- 将来追加する Provider の credential

すべて Cloud Run 側の環境変数または Secret Manager で管理する。

## 14.2 CORS

Cloud Run API は GitHub Pages の Origin のみ許可する。

例:

```text
https://<github-user-or-org>.github.io
```

ローカル開発時のみ以下を許可する。

```text
http://localhost:5173
```

## 14.3 Rate Limit

MVP でも簡易 rate limit は入れる。

方針:

- IP 単位の簡易 rate limit を backend middleware で実装する
- 制限値は環境変数で調整できるようにする
- 公開デモで濫用リスクが高い場合は demo token を追加する
- Cloud Armor は MVP 後の運用強化として扱う

## 14.4 画像サイズ制限

API 側で以下を制限する。

```text
最大リクエストサイズ: 2MB〜5MB
許可 MIME type: image/jpeg, image/png, image/webp
```

---

## 15. コスト管理

## 15.1 フロントエンド側

- ボタン押下式にする
- 枠内だけ切り出す
- 画像を縮小する
- JPEG/WebP 圧縮を行う
- 自動連続送信を行わない

## 15.2 バックエンド側

- Bedrock の利用モデルと出力 token 数を制限する
- 出力 token 数を制限する
- 検索結果数を 3〜5 件に制限する
- 検索結果全文を大量に LLM へ渡さない
- timeout を設定する
- エラー時の retry を過剰に行わない

## 15.3 Cloud Run 側

推奨初期設定:

```text
min instances: 0
max instances: small fixed number
CPU: 1
Memory: 512MiB or 1GiB
Concurrency: 10〜80
Timeout: 30〜60秒
```

デモ発表時のみ cold start 対策として `min instances: 1` を検討する。

---

## 16. デプロイ設計

## 16.1 Frontend

GitHub Actions で GitHub Pages にデプロイする。

```text
frontend build
  ↓
dist
  ↓
GitHub Pages
```

必要な環境変数:

```env
VITE_API_BASE_URL=https://your-cloud-run-service-url
```

## 16.2 Backend

GitHub Actions で Cloud Run にデプロイする。

```text
Go test
  ↓
Docker build
  ↓
Artifact Registry push
  ↓
Cloud Run deploy
```

Cloud Run に設定する環境変数例:

```env
APP_ENV=production
ALLOWED_ORIGINS=https://your-user.github.io
LLM_PROVIDER=bedrock
SEARCH_PROVIDER=tavily
AWS_REGION=us-east-1
BEDROCK_MODEL_ID=anthropic.claude-3-5-sonnet-20241022
TAVILY_API_KEY=xxx
RATE_LIMIT_PER_MINUTE=30
```

---

## 17. エラーハンドリング

## 17.1 共通エラーレスポンス

```json
{
  "error": {
    "code": "invalid_request",
    "message": "imageBase64 is required",
    "requestId": "req_01H..."
  }
}
```

## 17.2 エラーコード

| Code | Description |
|---|---|
| `invalid_request` | リクエスト不正 |
| `image_too_large` | 画像サイズ超過 |
| `unsupported_image_type` | 非対応画像形式 |
| `llm_error` | LLM 呼び出し失敗 |
| `search_error` | 検索 API 呼び出し失敗 |
| `timeout` | タイムアウト |
| `rate_limited` | 呼び出し制限 |
| `internal_error` | その他内部エラー |

---

## 18. ロギング

Cloud Logging に以下を出す。

- requestId
- endpoint
- elapsedMs
- llmProvider
- llmModel
- searchProvider
- searchResultCount
- errorCode
- image size metadata

画像そのものや base64 文字列はログに出さない。

---

## 19. 将来拡張

## 19.1 自動認識

MVP 後に以下の順で追加する。

```text
Phase 1:
  ボタン押下式

Phase 2:
  cooldown付き一定間隔認識

Phase 3:
  ブラウザ内画像差分検知

Phase 4:
  ブラウザ内軽量物体検出
```

自動認識追加時の制約:

- API 呼び出し間隔の最小値を設定する
- 同一画像・同一検索クエリの再送を抑制する
- セッション単位で最大呼び出し数を設定する
- API 処理中は次の送信を行わない

## 19.2 検索結果の保存

将来的には以下に保存できる。

- Cloud Storage
- Firestore
- BigQuery
- PostgreSQL
- Vector DB

保存対象は `Search Artifact` 形式を基本とする。

## 19.3 次プロジェクトへの接続例

検索結果は以下のようなプロジェクトへ接続しやすい。

- 画像から自動で調査メモを作るアプリ
- 物体認識ベースの商品比較アプリ
- 検索結果をナレッジベース化する RAG アプリ
- フィールドワーク用の観察ログアプリ
- 画像から関連資料を収集するリサーチ支援ツール
- URL と要約を蓄積するブックマーク拡張

## 19.4 Provider 追加

将来的に以下を追加可能。

LLM:

- Google Cloud Vertex AI
- OpenAI
- Anthropic direct API
- Azure OpenAI
- ローカル VLM

Search:

- Brave Search API
- Serper
- Google Custom Search
- Bing Web Search
- Exa
- Perplexity API
- 独自クローラー

---

## 20. MVP 実装順序

推奨順序:

```text
1. リポジトリ作成
2. frontend Vite React セットアップ
3. カメラ表示
4. 枠内 crop
5. backend Go API セットアップ
6. /healthz 実装
7. /api/recognize-search の request/response 仮実装
8. mock LLM 実装
9. mock search 実装
10. frontend と backend 接続
11. Bedrock adapter 実装
12. Tavily search adapter 実装
13. 検索結果 normalizer 実装
14. 要約処理実装
15. rate limit / request size validation / CORS 実装
16. Cloud Run deploy
17. GitHub Pages deploy
18. README / docs 整備
```

---

## 21. 決定事項

- MVP はボタン押下式とする
- フロントエンドは GitHub Pages にデプロイする
- バックエンドは Cloud Run にデプロイする
- バックエンド言語は Go とする
- MVP の LLM Provider は Bedrock に固定する
- MVP の Search Provider は Tavily に固定する
- LLM は Provider Interface で抽象化し、MVP 後の Provider 追加に備える
- 検索 API も Provider Interface で抽象化し、MVP 後の Provider 追加に備える
- 検索結果は次プロジェクトに繋げやすい共通スキーマに正規化する
- MVP では検索履歴の永続保存は行わない

---

## 22. 未決定事項

- Cloud Run のリージョン
  - GCP プロジェクトの設定に合わせて決定する

- UI デザイン
  - MVP ではシンプルなデモ UI とする

- 認識対象カテゴリ
  - MVP では一般物体とする

---

## 23. 推奨 MVP 構成まとめ

```text
Frontend:
  React + TypeScript + Vite
  GitHub Pages

Backend:
  Go
  Cloud Run

LLM:
  Amazon Bedrock fixed for MVP

Search:
  Tavily fixed for MVP

Recognition:
  Button-triggered capture only

Data:
  Normalized Search Result
  Search Artifact compatible schema
```