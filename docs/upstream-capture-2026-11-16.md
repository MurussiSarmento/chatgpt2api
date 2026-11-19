# 2026-11-16 Real Packet Capture of the ChatGPT Web Client

This document records a real ChatGPT web client request from a live Chrome session, used for later investigation of upstream protocol changes. Only the core request/response structure and fingerprint headers are kept; all sensitive values such as `Authorization`, Cookie, sentinel token, `conduit_token`, account id, and email are masked or replaced with placeholders.

## Environment

| Item | Value |
|:--|:--|
| Capture time | 2026-11-16 10:45 (Asia/Shanghai) |
| Browser | Chrome 157.0.8103.51 (Windows 11 x64) |
| Pages | `https://chatgpt.com/`, `https://chatgpt.com/images` |
| Page `data-build` | `5750d92c02e7d1b6aa2ea1ffdb1c1fb376272952` |
| Account plan | free |
| Capture method | Standalone Chrome profile + CDP `Network.requestWillBeSent`, triggered by real clicks/typing |
| Default model | Request `model: "auto"`, response `model_slug: "gpt-6-t-mini"` |

## Core Request Headers

Carried by both text-conversation and image-generation requests:

```http
authorization: Bearer <redacted>
chatgpt-account-id: <redacted-account-uuid>
oai-did: <device-uuid>
oai-device-id: <device-uuid>
oai-language: zh-CN
originator: Codex Browser
x-openai-web-frontend: codex_webview
x-openai-codex-window-type: browser
x-openai-rate-limit-join-workspace: 1
x-openai-rejoin-pro-cta: 1
x-oai-mcp-form-version: 2
x-oai-turn-trace-id: <turn-trace-uuid>
content-type: application/json
origin: https://chatgpt.com
referer: https://chatgpt.com/
sec-ch-ua: "Not-A.Brand";v="99", "Google Chrome";v="157", "Chromium";v="157"
sec-ch-ua-mobile: ?0
sec-ch-ua-platform: "Windows"
user-agent: Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/157.0.0.0 Safari/537.36
```

The official `/f/conversation` additionally carries:

```http
x-conduit-token: <redacted>
openai-sentinel-chat-requirements-token: <redacted>
openai-sentinel-proof-token: <redacted>
openai-sentinel-turnstile-token: <redacted>
x-openai-web-sse-compression: gzip
```

Notes:

- `x-openai-web-sse-compression: gzip` is a web-client experiment switch; a reverse-proxy implementation can omit it and simply read the SSE as uncompressed.
- `x-openai-genui-client-actions`, `x-openai-beacon-*`, and `oai-echo-logs` are frontend telemetry / component capabilities and do not affect the main conversation route.
- The web client sends both `oai-did` and `oai-device-id` with identical values.

## Text Conversation

### 1. `POST /backend-api/f/conversation/prepare`

The prepare request is a trimmed payload, without `messages` or `partial_query`:

```json
{
  "client_prepare_state": "sent",
  "action": "next",
  "model": "auto",
  "parent_message_id": "client-created-root",
  "timezone": "Asia/Shanghai",
  "timezone_offset_min": -480,
  "is_do_not_remember": false,
  "local_function_names": ["local.continue_in_work"],
  "client_prepare_dispatch": "debounced",
  "client_prepare_source": "composer_editor_state"
}
```

Core response fields:

```json
{
  "status": "ok",
  "conduit_token": "<redacted-jwt>"
}
```

### 2. `POST /backend-api/f/conversation`

```json
{
  "turn_attribution": { "turn_trigger": "composer" },
  "action": "next",
  "model": "auto",
  "parent_message_id": "client-created-root",
  "timezone": "Asia/Shanghai",
  "timezone_offset_min": -480,
  "is_do_not_remember": false,
  "client_contextual_info": {
    "app_name": "chatgpt.com",
    "app_surface": "codex_browser",
    "has_web_push_capabilities": true,
    "web_push_notification_permission": "default"
  },
  "local_function_names": ["local.continue_in_work"],
  "messages": [
    {
      "author": { "metadata": {}, "name": null, "role": "user" },
      "channel": null,
      "content": { "content_type": "text", "parts": ["只回复两个字：收到"] },
      "create_time": 1794797126.795,
      "end_turn": null,
      "id": "88cc7961-6275-420b-9356-696c60b60480",
      "metadata": {
        "automation_creation_attribution": {
          "origin": "conversation",
          "flow_id": "88cc7961-6275-420b-9356-696c60b60480"
        }
      },
      "recipient": "all",
      "status": "finished_successfully",
      "update_time": null,
      "weight": 1
    }
  ],
  "supported_encodings": ["v1"],
  "genui_state_snapshots": [],
  "client_prepare_state": "success"
}
```

Key points:

- An ordinary text conversation has **no** `conversation_mode` and **no** empty `system_hints`.
- `turn_attribution` and `genui_state_snapshots` **are still present** — do not remove them.
- `author.name` is `null`, and `author.metadata` is an empty object.
- `metadata.automation_creation_attribution.flow_id` is the same as the message `id`.
- `messages` contains only this turn's user message; for a new conversation `parent_message_id` is `client-created-root`.

### 3. `GET /backend-api/conversations/{id}?num_turns=10&include_has_versions=true`

The real web client reads conversation details using the **plural** endpoint, returning a `messages` array:

```json
{
  "conversation_id": "<conversation-id>",
  "default_model_slug": "auto",
  "current_node": "<assistant-message-id>",
  "messages": [
    {
      "id": "<system-message-id>",
      "author": { "role": "system", "name": null, "metadata": {} },
      "content": { "content_type": "text", "parts": [""] },
      "status": "finished_successfully",
      "metadata": {
        "system_hints": ["reason"],
        "is_visually_hidden_from_conversation": true
      },
      "recipient": "all",
      "channel": null
    },
    {
      "id": "<user-message-id>",
      "author": { "role": "user", "name": null, "metadata": {} },
      "content": { "content_type": "text", "parts": ["只回复两个字：收到"] },
      "status": "finished_successfully",
      "weight": 1.0,
      "recipient": "all",
      "channel": null
    },
    {
      "id": "<assistant-message-id>",
      "author": { "role": "assistant", "name": null, "metadata": {} },
      "content": { "content_type": "text", "parts": ["收到"] },
      "status": "finished_successfully",
      "end_turn": true,
      "weight": 1.0,
      "metadata": {
        "finish_details": { "type": "stop", "stop_tokens": [200002] },
        "is_complete": true,
        "model_slug": "gpt-6-t-mini",
        "default_model_slug": "auto"
      },
      "recipient": "all",
      "channel": "final"
    }
  ],
  "page_info": {
    "start_cursor": "<system-message-id>",
    "end_cursor": "<assistant-message-id>",
    "has_previous_page": false,
    "has_next_page": false
  }
}
```

## Image-Generation Conversation

### 1. `POST /backend-api/f/conversation/prepare`

One more `system_hints` than text:

```json
{
  "client_prepare_state": "sent",
  "action": "next",
  "model": "auto",
  "parent_message_id": "client-created-root",
  "timezone": "Asia/Shanghai",
  "timezone_offset_min": -480,
  "is_do_not_remember": false,
  "system_hints": ["picture_v2"],
  "local_function_names": ["local.continue_in_work"],
  "client_prepare_dispatch": "debounced",
  "client_prepare_source": "composer_editor_state"
}
```

### 2. `POST /backend-api/f/conversation`

```json
{
  "turn_attribution": { "turn_trigger": "composer" },
  "action": "next",
  "model": "auto",
  "parent_message_id": "client-created-root",
  "timezone": "Asia/Shanghai",
  "timezone_offset_min": -480,
  "is_do_not_remember": false,
  "system_hints": ["picture_v2"],
  "client_contextual_info": {
    "app_name": "chatgpt.com",
    "app_surface": "codex_browser",
    "has_web_push_capabilities": true,
    "web_push_notification_permission": "default"
  },
  "local_function_names": ["local.continue_in_work"],
  "messages": [
    {
      "author": { "metadata": {}, "name": null, "role": "user" },
      "channel": null,
      "content": {
        "content_type": "text",
        "parts": ["一个红色圆形图标，白色背景，极简风格"]
      },
      "create_time": 1794797173.206,
      "end_turn": null,
      "id": "66811a82-3d46-4c51-95c7-8596720896b7",
      "metadata": {
        "automation_creation_attribution": {
          "origin": "conversation",
          "flow_id": "66811a82-3d46-4c51-95c7-8596720896b7"
        },
        "system_hints": ["picture_v2"]
      },
      "recipient": "all",
      "status": "finished_successfully",
      "update_time": null,
      "weight": 1
    }
  ],
  "supported_encodings": ["v1"],
  "genui_state_snapshots": [],
  "client_prepare_state": "success"
}
```

Key points:

- For image generation, `system_hints: ["picture_v2"]` appears at **both** the **top level** and in the **user message metadata**.
- When no size/count is selected, the request has **no** `image_generation_settings`; only after selecting an aspect ratio/count is it written into the user message metadata.
- The image-generation result is not a separate endpoint but a subsequent `role=tool` message in the same conversation.

### 3. `POST /backend-api/conversation/init`

The image-generation page additionally calls init, with `system_hints` set to `["picture_v2"]`:

```json
{
  "conversation_id": null,
  "conversation_origin": null,
  "gizmo_id": null,
  "requested_default_model": null,
  "system_hints": ["picture_v2"],
  "timezone": "Asia/Shanghai",
  "timezone_offset_min": -480
}
```

For ordinary text, init sets `system_hints` to `null`:

```json
{
  "conversation_id": null,
  "conversation_origin": null,
  "gizmo_id": null,
  "requested_default_model": null,
  "system_hints": null,
  "timezone": "Asia/Shanghai",
  "timezone_offset_min": -480
}
```

## Image-to-Image / Image Editing

### 1. `POST /backend-api/f/conversation/prepare`

A prepare request with an image additionally passes the attachment MIME types and the already-uploaded file IDs:

```json
{
  "attachment_mime_types": ["image/png"],
  "prepare_image_upload_ids": ["file_<upload-id-1>", "file_<upload-id-2>"],
  "client_prepare_state": "sent",
  "action": "next",
  "model": "auto",
  "parent_message_id": "client-created-root",
  "timezone": "Asia/Shanghai",
  "timezone_offset_min": -480,
  "is_do_not_remember": false,
  "system_hints": ["picture_v2"],
  "local_function_names": ["local.continue_in_work"],
  "client_prepare_dispatch": "debounced",
  "client_prepare_source": "composer_editor_state"
}
```

### 2. `POST /backend-api/f/conversation`

A message with an image is `multimodal_text`, with the **image pointer first and the text last**:

```json
{
  "turn_attribution": { "turn_trigger": "composer" },
  "action": "next",
  "model": "auto",
  "parent_message_id": "client-created-root",
  "timezone": "Asia/Shanghai",
  "timezone_offset_min": -480,
  "is_do_not_remember": false,
  "system_hints": ["picture_v2"],
  "client_contextual_info": {
    "app_name": "chatgpt.com",
    "app_surface": "codex_browser",
    "has_web_push_capabilities": true,
    "web_push_notification_permission": "default"
  },
  "local_function_names": ["local.continue_in_work"],
  "messages": [
    {
      "author": { "metadata": {}, "name": null, "role": "user" },
      "channel": null,
      "content": {
        "content_type": "multimodal_text",
        "parts": [
          {
            "asset_pointer": "sediment://file_<upload-id>",
            "content_type": "image_asset_pointer",
            "height": 1086,
            "size_bytes": 994361,
            "width": 1448
          },
          "把背景改成蓝色"
        ]
      },
      "create_time": 1794797449.894,
      "end_turn": null,
      "id": "<message-id>",
      "metadata": {
        "automation_creation_attribution": {
          "origin": "conversation",
          "flow_id": "<message-id>"
        },
        "attachments": [
          {
            "height": 1086,
            "id": "file_<upload-id>",
            "library_file_id": "libfile_<library-id>",
            "mime_type": "image/png",
            "name": "source.png",
            "size": 994361,
            "source": "local",
            "width": 1448
          }
        ]
      },
      "recipient": "all",
      "status": "finished_successfully",
      "update_time": null,
      "weight": 1
    }
  ],
  "supported_encodings": ["v1"],
  "genui_state_snapshots": [],
  "client_prepare_state": "success"
}
```

Key points:

- The image part itself has **no** `mime_type`; the MIME type is placed in the message's `metadata.attachments[].mime_type`.
- The user-uploaded image pointer is `sediment://file_*`, not `file-service://`.
- Uploading a file goes through `/backend-api/files` to get an `upload_url`; after the PUT, confirm with `/backend-api/files/{id}/uploaded`.
- `prepare_image_upload_ids` passes the `file_id` returned by this turn's upload; `attachment_mime_types` is deduplicated and sorted.

## Continued-Conversation prepare

When continuing to send messages in the same conversation, prepare carries the real `conversation_id` and `parent_message_id`, and changes dispatch/source to an immediate context change:

```json
{
  "client_prepare_state": "sent",
  "conversation_id": "<conversation-id>",
  "action": "next",
  "model": "auto",
  "parent_message_id": "<last-message-id>",
  "timezone": "Asia/Shanghai",
  "timezone_offset_min": -480,
  "is_do_not_remember": false,
  "local_function_names": ["local.continue_in_work"],
  "client_prepare_dispatch": "immediate",
  "client_prepare_source": "context_change"
}
```

## SSE Event Order

The typical order of a normal text turn:

```text
data: "v1"
data: {"type":"resume_conversation_token","kind":"topic","token":"<redacted>","conversation_id":"<id>"}
data: {"p":"","o":"add","v":{"message":{"author":{"role":"system"},"metadata":{"is_visually_hidden_from_conversation":true,...}}}}
data: {"v":{"message":{"author":{"role":"user"},"content":{"parts":["..."]},"recipient":"all","channel":null}}}
data: {"type":"input_message","input_message":{...}}
data: {"type":"title_generation","title":"...","conversation_id":"<id>"}
...several hidden role=system messages (rebase / developer / plugin upsell)...
data: {"v":{"message":{"author":{"role":"assistant"},"content":{"content_type":"text","parts":[""]},"status":"in_progress","channel":"final"}}}
data: {"type":"message_marker","marker":"user_visible_token","event":"first"}
data: {"p":"/message/content/parts/0","o":"append","v":"收到"}
data: {"o":"patch","v":[{"p":"/message/content/parts/0","o":"append","v":"..."},{"p":"/message/end_turn","o":"replace","v":null}]}
data: {"v":[{"p":"/message/status","o":"replace","v":"finished_successfully"},{"p":"/message/end_turn","o":"replace","v":true}]}
data: {"type":"message_marker","marker":"last_token","event":"last"}
data: {"type":"message_stream_complete",...}
data: {"type":"user_signal_metadata",...}
```

Parsing notes:

- `conversation_id` may appear at the top level or inside `v`; take the first occurrence.
- Only accept messages where `author.role == "assistant"`, `channel` is empty or `final`, and which are not `is_visually_hidden_from_conversation`.
- Text may be incrementally updated via `append`/`replace` at `/message/content/parts/0`.
- `message_stream_complete` may appear before `[DONE]`.

## Voice List

The real web client does not hardcode the voices in the frontend; it requests them:

```http
GET /backend-api/settings/voices?spoken_language=zh-CN&voice_mode=advanced
Authorization: Bearer <token>
ChatGPT-Account-ID: <account-id>
```

Response structure:

```json
{
  "selected": "juniper",
  "voices": [
    {
      "voice": "juniper",
      "name": "Juniper",
      "description": "Open and upbeat",
      "accent": "American English",
      "preview_url": "https://persistent.oaistatic.com/voice/previews/juniper.zh.<hash>.m4a"
    }
  ]
}
```

`advanced` / `standard` / `zh-CN` / `en-US` currently all return the same set of 9 voices:

| `voice` ID | Display name | Upstream description | Accent |
|:--|:--|:--|:--|
| `juniper` | Juniper | Open and upbeat | American English |
| `maple` | Maple | Cheerful and candid | American English |
| `cove` | Cove | Composed and direct | American English |
| `breeze` | Breeze | Animated and earnest | American English |
| `orbit` | **Spruce** | Calm and affirming | American English |
| `glimmer` | **Sol** | Savvy and relaxed | American English |
| `ember` | Ember | Confident and optimistic | American English |
| `fathom` | **Arbor** | Easygoing and versatile | British English |
| `vale` | Vale | Bright and inquisitive | British English |

Notes:

- The upstream ID and display name do not fully match: `orbit` = Spruce, `glimmer` = Sol, `fathom` = Arbor.
- The `voice` parameter of `/backend-api/synthesize` accepts the official website IDs above, and also accepts OpenAI voice names (`alloy`, `ash`, `coral`, `echo`, `fable`, `onyx`, `nova`, `sage`, `shimmer`).
- Passing a non-existent voice returns `HTTP 404 {"detail":{"message":"Voice not found","code":"voice_not_found"}}`, which can be used to validate whether a voice actually exists.
- `voice_mode=wingman` additionally returns real-time voices such as `bubbie`, `faro`, `tonada`, `rio`, `viola`, which ordinary reading/synthesis does not use.

## Image-Generation Result Message

The `messages` of `GET /backend-api/conversations/{id}` will include:

```json
{
  "id": "<tool-message-id>",
  "author": { "role": "tool", "name": "t2uay3k.sj1i4kz", "metadata": {} },
  "content": {
    "content_type": "multimodal_text",
    "parts": [
      {
        "content_type": "image_asset_pointer",
        "asset_pointer": "sediment://file_<id>",
        "mime_type": "image/png",
        "size_bytes": 1041922,
        "width": 1254,
        "height": 1254,
        "metadata": {
          "generation": {
            "gen_id": "<gen-id>",
            "height": 1254,
            "width": 1254,
            "orientation": "square"
          }
        }
      }
    ]
  },
  "status": "finished_successfully",
  "metadata": {
    "image_gen_title": "简约红苹果插画",
    "permissions": [
      {
        "type": "notification",
        "status": "requested",
        "notification_channel_id": "image_gen"
      }
    ],
    "model_slug": "gpt-6",
    "default_model_slug": "auto"
  },
  "recipient": "all",
  "channel": "commentary"
}
```

Recognition rules:

- `author.role == "tool"`
- `author.name == "image_gen"` or starts with `t2uay3k`
- The image part's `metadata.generation` is non-empty
- `asset_pointer` is `sediment://file_*` or `file-service://file_*`

Download:

```http
GET /backend-api/files/download/{file_id}?download_intent=false&inline=true&conversation_id={conversation_id}
```

The `download_url` in the returned JSON points to `https://chatgpt.com/backend-api/estuary/content?...`; download the binary with the Bearer Token attached.
