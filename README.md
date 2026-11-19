<h1 align="center">ChatGPT2API</h1>


<p align="center">ChatGPT2API is primarily a reverse-engineered organization and wrapper of the capabilities of the official ChatGPT website. It provides an OpenAI-compatible image API / proxy targeting ChatGPT image generation, image editing, and multi-image composition editing scenarios, and integrates an online drawing studio, account-pool management, multiple account import methods, and Docker self-hosted deployment. Built with Go, it supports high concurrency with an extremely low memory footprint — only about 25MB when idle.</p>

> [!WARNING]
> Disclaimer:
>
> This project involves reverse-engineering research on interfaces related to text generation, image generation, and image editing on the official ChatGPT website. It is intended solely for personal study, technical research, and non-commercial technical exchange.
>
> - Using this project for any commercial purpose, profit-making use, bulk operations, automated abuse, or large-scale invocation is strictly prohibited.
> - Using this project to disrupt market order, engage in malicious competition, arbitrage reselling, secondary reselling of related services, or any behavior that violates OpenAI's Terms of Service or local laws and regulations is strictly prohibited.
> - Using this project to generate, disseminate, or assist in generating illegal, violent, pornographic, or minor-related content, or for fraud, scams, harassment, or any other illegal or improper purposes is strictly prohibited.
> - Users assume all risks themselves, including but not limited to account restrictions, temporary bans, permanent bans, and any legal liability resulting from improper use.
> - Using this project is deemed full understanding of and agreement to the entire contents of this disclaimer; the user bears sole responsibility for any consequences arising from abuse, violations, or illegal use.
> - This project is implemented based on reverse-engineering research of the official ChatGPT website's capabilities, and carries the risk of account restrictions, temporary bans, or permanent bans. Do not use your own important, frequently used, or high-value accounts for testing.


## Sponsors

<table>
  <tr>
    <td width="190" align="center">
      <a href="https://kunrouai.com/?utm_source=chatgpt2api"><img src="assets/kunrouai.svg" width="163" alt="KunrouAI"></a>
    </td>
    <td>
      <a href="https://kunrouai.com/?utm_source=chatgpt2api">KunrouAI</a> is an all-in-one AI visual creation platform combining AI image generation, video generation, multi-user collaborative canvas, and Agent/MCP connectivity. It suits both individual creators and team collaboration, and offers source-code sales and private deployment services for enterprises — enterprise inquiries and partnerships are welcome! There is also an <a href="https://api.kunrouai.com/?utm_source=chatgpt2api">API open platform</a> offering a rich set of AI model API interfaces with fast integration — developers and enterprises are welcome to collaborate!
    </td>
  </tr>
  <tr>
    <td width="190" align="center">
      <a href="https://88api.ai/sign-up?aff=py0a"><img src="assets/88api.png" width="163" alt="88API"></a>
    </td>
    <td>
      <a href="https://88api.ai/sign-up?aff=py0a">88API</a> is an all-in-one multi-model API platform operated by a Hong Kong company, aimed primarily at developers, creators, and AI application users. Through a unified interface it provides access to text, image, speech, and video models, covering common workflows such as AI coding, intelligent translation, content creation, voiceover, image generation, and video generation. It supports adding and switching Providers within popular clients, supports major international payment methods, can provide invoices, and offers enterprise-grade stable service. Register via <a href="https://88api.ai/sign-up?aff=py0a">this link</a> to receive an exclusive bonus credit!
    </td>
  </tr>
  <tr>
    <td width="190" align="center">
      <a href="https://www.ipwo.net/?ref=githubbasketikunchatgpt2api"><img src="assets/ipwo.png" width="163" alt="IPWO"></a>
    </td>
    <td>
      <a href="https://www.ipwo.net/?ref=githubbasketikunchatgpt2api">IPWO</a> residential proxies offer dynamic, static, and unlimited residential proxies, covering 195+ countries and regions, supporting HTTP(S) and SOCKS5 protocols.<br>For developers using ChatGPT2API for AI image generation, image editing, and API integration, IPWO offers free testing, supports region selection and network connection configuration, and meets a variety of development and testing needs. Discount code: <b>0200</b>.
    </td>
  </tr>
  <tr>
    <td width="190" align="center">
      <a href="https://www.atlascloud.ai/zh?utm_source=github&utm_medium=link&utm_campaign=chatgpt2api"><img src="assets/atlascloud.svg" width="163" alt="Atlas Cloud"></a>
    </td>
    <td>
      <a href="https://www.atlascloud.ai/zh?utm_source=github&utm_medium=link&utm_campaign=chatgpt2api">Atlas Cloud</a> is a full-modal AI inference platform that gives developers a single AI API to access video generation, image generation, and LLM APIs. Instead of managing multiple vendor integrations, you connect once and get unified access to 300+ curated models across all modalities. Check out <a href="https://www.atlascloud.ai/console/coding-plan">Atlas Cloud's new coding plan promotion</a> for more budget-friendly API access.
    </td>
  </tr>
</table>

## Quick Start

### Running with Docker

```bash
git clone git@github.com:basketikun/chatgpt2api.git
cd chatgpt2api
docker compose up -d
```

Before starting, set `api-key` in `config.yaml`; you can also override it via `CHATGPT2API_AUTH_KEY` in `.env`.

- Web panel: `http://localhost:3000`
- API endpoint: `http://localhost:3000/v1`
- Data directory: `./data`

### Local Development

```bash
go run .
```

The frontend lives in `web/` and requires no build step — simply access it once the service is running.

For deployment and upgrades, see [docs/deployment.md](docs/deployment.md).

## Features

### API Compatibility

- Compatible `POST /v1/images/generations` image generation endpoint
- Compatible `POST /v1/images/edits` image editing endpoint
- Compatible `POST /v1/audio/speech` speech synthesis endpoint (the official ChatGPT website's voice model)
- Compatible `POST /v1/audio/transcriptions` speech transcription endpoint
- Compatible `POST /v1/chat/completions` for image-oriented scenarios
- Compatible `POST /v1/responses` for image-oriented scenarios
- `GET /v1/models` aggregates the official models available across accounts; common values include `auto`, `gpt-6`, `gpt-6-mini`, `gpt-6-t-mini`,
  `gpt-5-6`, `gpt-5-6-mini`, `gpt-5.6-terra-wm`, `gpt-5.6-luna-wm`, `research`, plus the image-generation alias `gpt-image-2.6`
- Supports returning multiple generated results via `n`
- Supports reverse-engineering of the image-drawing interface in Codex, available only to `Plus` / `Team` / `Pro` subscriptions; externally it still uniformly uses `gpt-image-2.6`, while the subscription tier and route (Web / Codex) selected on the user key determine which class of account is used.
  The same account has both official-website and Codex image-generation quotas simultaneously; the Codex route defaults to the main-control model `gpt-6-luna` and the drawing tool model `gpt-image-2.6`, both adjustable on the settings page.

### Creation Studio

- Built-in creation studio supporting chat (web search, image Q&A), image generation and editing, and speech synthesis and transcription
- Image generation supports quality, size, count, and reference-image settings; tasks can continue in the background and keep waiting after a timeout
- Creation history is saved server-side and can be reviewed, deleted, and cleared
- Non-admin keys go straight to the creation studio after logging in

### Account-Pool Management

- Automatically refreshes account email, type, quota, and recovery time (asynchronous progress tracking)
- Polls available accounts to perform image generation and image editing
- Automatically removes invalid tokens when encountering token-expiry errors
- Periodically checks rate-limited accounts and refreshes them automatically
- Supports re-login with password to recover abnormal accounts; can auto-relogin after a refresh
- Supports configuring a global HTTP / HTTPS / SOCKS5 / SOCKS5H proxy from the web UI
- Supports WARP / FlareSolverr stable proxy runtimes
- Supports search, filtering, batch refresh, export, manual editing, and cleanup of accounts
- Supports four import methods: local CPA JSON file import, remote CPA server import, `sub2api` server import, and `access_token` import
- Supports configuring a `sub2api` server on the settings page to filter and batch-import the OpenAI OAuth accounts it holds

## Showcase

<table width="100%">
  <tr>
    <td width="50%"><img src="https://i.ibb.co/Jj8nfwwP/image.png" alt="image" border="0"></td>
    <td width="50%"><img src="https://i.ibb.co/pqf235v/image-edit.png" alt="image edit" border="0"></td>
  </tr>
  <tr>
    <td width="50%"><img src="https://i.ibb.co/tPcqtVfd/chery-studio.png" alt="chery studio" border="0"></td>
    <td width="50%"><img src="https://i.ibb.co/PsT9YHBV/account-pool.png" alt="account pool" border="0"></td>
  </tr>
  <tr>
    <td width="50%"><img src="https://i.ibb.co/rRWLG08q/new-api.png" alt="new api" border="0"></td>
  </tr>
</table>

## API

All AI endpoints require the request header:

```http
Authorization: Bearer <api-key>
```

<details>
<summary><code>GET /v1/models</code></summary>
<br>

Returns the list of image models currently exposed.

```bash
curl http://localhost:8000/v1/models \
  -H "Authorization: Bearer <api-key>"
```

<details>
<summary>Details</summary>
<br>

| Field          | Description                                                                                                 |
|:---------------|:------------------------------------------------------------------------------------------------------------|
| Returned models | Based on the real-time response of `/v1/models`; common values: `auto`, `gpt-6`, `gpt-6-mini`, `gpt-5-6`, `gpt-5-6-mini`, `gpt-5.6-terra-wm`, `gpt-5.6-luna-wm`, `research`, `gpt-image-2.6` |
| Integration scenarios | Can be integrated with upstreams or clients such as Cherry Studio, New API, etc.                     |

<br>
</details>
</details>

<details>
<summary><code>POST /v1/images/generations</code></summary>
<br>

OpenAI-compatible image generation endpoint, used for text-to-image.

```bash
curl http://localhost:8000/v1/images/generations \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <api-key>" \
  -d '{
    "model": "gpt-image-2.6",
    "prompt": "a cat floating in outer space",
    "n": 1,
    "response_format": "b64_json"
  }'
```

<details>
<summary>Field descriptions</summary>
<br>

| Field             | Description                                              |
|:------------------|:---------------------------------------------------------|
| `model`           | Image model; currently available values are per the `/v1/models` response — `gpt-image-2.6` is recommended |
| `prompt`          | Image generation prompt                                  |
| `n`               | Number of images to generate; the backend currently limits this to `1-4` |
| `size`            | Optional, e.g. `1024x1024`, `1536x1024`; converted to the upstream `aspect_ratio` |
| `quality`         | Optional, `auto` / `low` / `medium` / `high`, etc., passed to the model as a quality hint |
| `response_format` | Included in the request model; defaults to `b64_json`    |

<br>
</details>
</details>

<details>
<summary><code>POST /v1/images/edits</code></summary>
<br>

OpenAI-compatible image editing endpoint. You can upload image files, or pass image links in the official JSON format and generate the edited result.

```bash
curl http://localhost:8000/v1/images/edits \
  -H "Authorization: Bearer <api-key>" \
  -F "model=gpt-image-2.6" \
  -F "prompt=turn this image into a cyberpunk night-scene style" \
  -F "n=1" \
  -F "image=@./input.png"
```

You can also pass an image URL directly:

```bash
curl http://localhost:8000/v1/images/edits \
  -H "Authorization: Bearer <api-key>" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-image-2.6",
    "prompt": "turn this image into a cyberpunk night-scene style",
    "images": [
      {"image_url": "https://example.com/input.png"}
    ]
  }'
```

<details>
<summary>Field descriptions</summary>
<br>

| Field       | Description                                        |
|:------------|:---------------------------------------------------|
| `model`     | Image model, `gpt-image-2.6`                        |
| `prompt`    | Image editing prompt                               |
| `n`         | Number of images to generate; the backend currently limits this to `1-4` |
| `image`     | The image file to edit, uploaded via multipart/form-data |
| `images`    | JSON array of image references, supporting `{"image_url": "https://..."}` |
| `image_url` | In form mode you can also pass an image link directly; repeat the field to pass multiple images |

<br>
</details>
</details>

<details>
<summary><code>POST /v1/chat/completions</code></summary>
<br>

A Chat Completions-compatible endpoint for text, web search, and image scenarios — not a full general-purpose chat proxy.

```bash
curl http://localhost:8000/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <api-key>" \
  -d '{
    "model": "gpt-image-2.6",
    "messages": [
      {
        "role": "user",
        "content": "generate a cyberpunk cat on a rainy night in the streets of Tokyo"
      }
    ],
    "n": 1
  }'
```

<details>
<summary>Field descriptions</summary>
<br>

| Field                | Description                                                                   |
|:---------------------|:------------------------------------------------------------------------------|
| `model`              | Text, search, or image model; a search model triggers the web-search compatibility logic |
| `messages`           | Message array, supporting text, search, and image request content            |
| `n`                  | Number of images to generate; parsed as the image count by the current implementation |
| `stream`             | Supported in text, search, and image scenarios; still under testing          |
| `tools`              | For text scenarios, supports `web_search` / `web_search_preview` / `web_search_preview_2025_03_11` |
| `web_search_options` | When provided, triggers the web-search compatibility logic                   |
| `thinking_effort`    | Optional; passed through per account/model as tiers like `low` / `medium` / `high` / `standard` / `extended` / `xhigh` / `max` / `ultra`; `auto` means it is not sent. Different plans unlock different tiers |

<br>
</details>
</details>

<details>
<summary><code>POST /v1/responses</code></summary>
<br>

A Responses API-compatible endpoint for text, web search, and image-generation tool calls — not a full general-purpose Responses API proxy.

```bash
curl http://localhost:8000/v1/responses \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <api-key>" \
  -d '{
    "model": "gpt-6",
    "input": "generate an image of a futuristic city skyline",
    "tools": [
      {
        "type": "image_generation"
      }
    ]
  }'
```

<details>
<summary>Field descriptions</summary>
<br>

| Field    | Description                                                                              |
|:---------|:----------------------------------------------------------------------------------------|
| `model`  | This model field is echoed back in the response; search and image generation go through their respective compatibility logic |
| `input`  | Input content; search uses the last user text, while image generation needs to be able to parse out a prompt |
| `tools`  | Supports `image_generation`, `web_search`, `web_search_preview`, `web_search_preview_2025_03_11` |
| `stream` | Implemented, but still under testing                                                    |

<br>
</details>
</details>

<details>
<summary><code>POST /v1/audio/speech</code></summary>
<br>

Calls the official ChatGPT website's voice model for speech synthesis, returning binary audio. Requires a logged-in account with voice capability such as `Plus` / `Team` / `Pro`.

```bash
curl http://localhost:8000/v1/audio/speech \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <api-key>" \
  -d '{
    "model": "gpt-tts",
    "input": "Hello, this is a speech synthesis test.",
    "voice": "alloy",
    "response_format": "mp3"
  }' \
  --output speech.mp3
```

<details>
<summary>Field descriptions</summary>
<br>

| Field             | Description                                                                                                             |
|:------------------|:-----------------------------------------------------------------------------------------------------------------------|
| `input`           | The text to synthesize, required                                                                                       |
| `voice`           | Official website voice IDs (`juniper`, `maple`, `cove`, `breeze`, `orbit`/Spruce, `glimmer`/Sol, `ember`, `fathom`/Arbor, `vale`); also compatible with OpenAI voice names (`alloy`, `ash`, `coral`, `echo`, `fable`, `onyx`, `nova`, `sage`, `shimmer`) |
| `response_format` | Audio format, defaults to `mp3`                                                                                        |
| `email`           | Optional, specifies a particular account from the pool to use                                                          |

<br>
</details>
</details>

<details>
<summary><code>POST /v1/audio/transcriptions</code></summary>
<br>

Calls the official ChatGPT website's speech-recognition interface for transcription; uploads an audio file via `multipart/form-data`.

```bash
curl http://localhost:8000/v1/audio/transcriptions \
  -H "Authorization: Bearer <api-key>" \
  -F "file=@speech.mp3" \
  -F "model=gpt-transcribe"
```

<details>
<summary>Details</summary>
<br>

| Field             | Description                                              |
|:------------------|:---------------------------------------------------------|
| `file`            | Audio file, required                                     |
| `response_format` | Pass `text` to return plain text; defaults to returning `{"text": "..."}` |
| `email`           | Optional, specifies a particular account from the pool to use |

<br>
</details>
</details>

## Community Support

Learn AI, visit the L site: [LinuxDO](https://linux.do)

## Contributors

Thanks to all the developers who have contributed to this project:

<a href="https://github.com/basketikun/chatgpt2api/graphs/contributors">
  <img alt="Contributors" src="https://contrib.rocks/image?repo=basketikun/chatgpt2api" />
</a>

## Star History

[![Star History Chart](https://api.star-history.com/chart?repos=basketikun/chatgpt2api&type=date&legend=top-left)](https://www.star-history.com/?repos=basketikun%2Fchatgpt2api&type=date&legend=top-left)
