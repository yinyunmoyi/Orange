# Orange

Orange 是一个面向英语词汇、句子和视频语境学习的本地应用，包含 Android 客户端、React Web 客户端和 Go 服务端。服务端负责词典、收藏、学习计划、语音合成、视频语境处理与局域网发现。

## 目录

```text
orange/
├── android/       # Kotlin + Jetpack Compose
├── web/           # React + Vite
├── server/        # Go + Hertz + GORM
├── docs/debug/    # 已脱敏的历史调试记录
├── .env.example   # 可公开配置模板
└── README.md
```

## 环境要求

- Go 1.20+
- Node.js 20.19+ 和 npm
- MySQL 8
- ECDICT SQLite 词典
- ffmpeg，可通过 `FFMPEG_BINARY` 指定路径
- Android Studio、JDK 17+ 和 Android SDK

## 初始化配置

在仓库根目录执行：

```bash
cp .env.example .env
```

`.env.example` 按当前实际部署所需配置编写，其中所有密钥、密码和公网域名均为假值。复制后逐项替换：

- `ANDROID_PUBLIC_BASE_URL`：Android 无法发现局域网服务时使用的公网服务 origin。当前项目通过 Zeronews 将本机 `8888` 端口暴露为 HTTPS 地址；也可使用其他内网穿透工具。该值会进入 APK，不能包含密钥。
- `WEB_BACKEND_URL`：Vite 将同源 `/api` 请求代理到本机 Go 服务，通常保持 `http://127.0.0.1:8888`。
- `WEB_ALLOWED_HOSTS`：允许访问 Vite 的主机名；使用公网穿透访问 Web 时加入穿透工具分配的域名。
- `MYSQL_DSN`：MySQL 8 连接串，来自自行创建的数据库账号。
- `ECDICT_PATH`：从 ECDICT Releases 下载并解压出的 `stardict.db` 路径。
- `LLM_API_KEY`：从 DeepSeek 开放平台创建。当前实现固定使用 DeepSeek Chat Completions 协议，默认调用 DeepSeek API 和 `deepseek-v4-pro` 模型，不支持替换为其他大模型厂商。
- `VOLC_TTS_API_KEY`、`VOLC_TTS_RESOURCE_ID`、`VOLC_TTS_SPEAKER`：从火山引擎语音技术控制台获取。当前实现固定使用火山引擎 TTS v3 单向流式接口和 Seed-TTS 2.0，不支持其他语音合成厂商。
- `AUDIO_DIR`：下载和合成后的音频存储目录。

其他监听端口、超时、日志和媒体目录使用代码中的安全默认值，需要定制时可通过同名环境变量覆盖。配置优先级为真实环境变量、根 `.env`、代码默认值。真实 `.env`、数据库、日志、媒体文件和个人学习数据不得提交。

## 数据库

先创建数据库和账号，再配置 `MYSQL_DSN`。例如：

```sql
CREATE DATABASE orange CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
```

初始化或更新表结构：

```bash
cd server
go run ./cmd/migrate
```

迁移命令可重复执行。服务正常启动不会自动修改表结构。

## 启动服务端

```bash
cd server
go mod download
go run .
```

可用 `go run . -port 9000` 临时覆盖默认端口。未配置 MySQL 时服务仍可启动，但收藏和学习相关功能不可用。未安装 ffmpeg 时视频 HLS 转封装不可用。

### 安装 ECDICT

ECDICT 是必需依赖。服务启动时会校验数据库文件、`stardict` 表及词条数量，缺失或损坏时直接退出。

1. 从 [skywind3000/ECDICT Releases](https://github.com/skywind3000/ECDICT/releases) 下载 SQLite 版本。
2. 解压得到 `stardict.db`。
3. 放到 `server/data/ecdict/stardict.db`，或通过 `ECDICT_PATH` 指定其他路径。

查词顺序如下：

1. 已收藏词优先读取 MySQL。
2. 普通查词和中文释义以本地 ECDICT 为主路径。
3. ECDICT 未收录词条时，英文信息回退到 `dictionaryapi.dev`。
4. ECDICT 缺少中文释义时，回退到 DeepSeek。
5. 公网词典仍失败时，仅返回火山引擎 TTS 发音兜底。

### 公网访问

当前部署使用 Zeronews 提供公网穿透地址，将本机 Go 服务的 `8888` 端口映射为 HTTPS origin，并填写到 `ANDROID_PUBLIC_BASE_URL`。Zeronews 不是代码依赖，ngrok、Cloudflare Tunnel 或其他能稳定转发 HTTP/SSE 的工具也可以替代。

长句分析使用 SSE 心跳避免公网网关因长时间无响应而超时。穿透工具需支持流式响应，且不应缓冲 SSE。

## 启动 Web

```bash
cd web
npm ci
npm run dev
```

浏览器请求使用同源 `/api`，Vite 根据根 `.env` 中的 `WEB_BACKEND_URL` 转发到服务端。

## 构建 Android

用 Android Studio 导入 `android/`，确保根 `.env` 已存在。命令行构建：

macOS 或 Linux：

```bash
cd android
./gradlew :app:assembleDebug
```

Windows：

```powershell
cd android
gradlew.bat :app:assembleDebug
```

APK 位于 `android/app/build/outputs/apk/debug/app-debug.apk`。

Android 优先通过 mDNS `_orange-context._tcp.` 发现局域网服务，并校验 `/ping` 返回的服务身份；失败后才使用 `ANDROID_PUBLIC_BASE_URL`。Android 14+ 需授予本地网络权限。防火墙需允许 UDP 5353 和服务端 TCP 端口，默认 8888。

## 测试

```bash
cd server && go test ./...
cd web && npm test && npm run lint && npm run build
cd android && ./gradlew :app:testDebugUnitTest :app:assembleDebug
```

## 常见问题

- Web `/api` 返回 502：确认 Go 服务已启动，且 `WEB_BACKEND_URL` 的端口正确。
- Android 无法发现服务：检查本地网络权限、多播、UDP 5353、防火墙和 `/ping` 身份校验。
- 视频转换失败：运行 `ffmpeg -version`，或设置 `FFMPEG_BINARY` 的绝对路径。
- ECDICT 启动失败：确认已下载 SQLite 版本、路径正确且数据库包含非空 `stardict` 表。
- DeepSeek 返回未配置：填写 DeepSeek 的 `LLM_API_KEY`；其他大模型厂商暂不支持。
- TTS 返回未配置：填写火山引擎 API Key、Seed-TTS 2.0 资源 ID 和英文音色；其他 TTS 厂商暂不支持。

## 不提交的本地内容

根 `.gitignore` 明确排除三个子工程中的 `.trae/`、`.idea/`、`.dbg/`、`.e2e/`，以及 `.env`、构建缓存、数据库、日志和媒体文件。`.trae/` 内是本地计划、截图和 Agent 过程资料，不属于发布候选文件，因此未逐份脱敏；后续初始化 Git 时也不应强制添加这些目录。

## 许可证

[MIT](LICENSE)
