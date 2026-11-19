# Upstream Conversation SSE Protocol Notes

Conversation SSE is the streaming return protocol of the upstream conversation route. Each SSE `data:` is usually a JSON payload, but may also be a protocol marker or an end marker. The client needs to consume these payloads in order, maintaining the current conversation state, text content, tool-call state, and image-result pointers.

## Request Payload (real browser capture, 2026-11-16)

Current fields submitted by `POST /backend-api/f/conversation`:

| Field | Description |
|:--|:--|
| `turn_attribution` | `{"turn_trigger":"composer"}` |
| `action` | Fixed `next` |
| `messages` | Message array; each message carries `author`, `content`, `metadata`, `channel`, `recipient`, `status`, `end_turn`, `weight`, `update_time` |
| `model` | Upstream model slug, e.g. `auto`, `gpt-6`, `gpt-6-mini` |
| `parent_message_id` | `client-created-root` for a new conversation |
| `supported_encodings` | `["v1"]` |
| `genui_state_snapshots` | `[]` |
| `client_prepare_state` | `success` after prepare succeeds |
| `system_hints` | Appears only in scenarios such as search / image generation, e.g. `["search"]`, `["picture_v2"]`, `["reason"]` |
| `thinking_effort` | Appears when the user selects a thinking effort; passed through per account/model as tiers like `low` / `medium` / `high` / `standard` / `extended` / `xhigh` / `max` / `ultra` |
| `client_contextual_info` | `app_name`, `app_surface`, `has_web_push_capabilities`, `web_push_notification_permission` |
| `is_do_not_remember` | Whether not to remember |
| `force_use_search` | `true` for a search turn |
| `timezone` / `timezone_offset_min` | Timezone and offset |

An ordinary text conversation **does not send** `conversation_mode`, nor does it **use** `supports_buffering`, `force_parallel_switch`, `paragen_cot_summary_display_override`, or `enable_message_followups`.

The prepare request `POST /backend-api/f/conversation/prepare` is a trimmed payload, without `messages` / `partial_query`:

```json
{"client_prepare_state":"sent","action":"next","model":"auto","parent_message_id":"client-created-root","timezone":"Asia/Shanghai","timezone_offset_min":-480,"is_do_not_remember":false,"system_hints":["picture_v2"],"local_function_names":["local.continue_in_work"],"client_prepare_dispatch":"debounced","client_prepare_source":"composer_editor_state"}
```

The `conduit_token` in the response needs to be carried back as `x-conduit-token` on the official request. When continuing a conversation, prepare additionally carries `conversation_id`, the real `parent_message_id`, `client_prepare_dispatch` of `immediate`, and `client_prepare_source` of `context_change`; when an image is included, it additionally carries `attachment_mime_types` (deduplicated and sorted) and `prepare_image_upload_ids` (the `file_id` uploaded this turn).

Related endpoints and request headers:

| Purpose | Method and path |
|:--|:--|
| Submit one conversation turn | `POST /backend-api/f/conversation` |
| Prepare / exchange for a conduit token | `POST /backend-api/f/conversation/prepare` |
| Read conversation details | `GET /backend-api/conversations/{conversation_id}?num_turns=10&include_has_versions=true` |
| Read conversation init / quota | `POST /backend-api/conversation/init` |
| Delete a conversation | `DELETE /backend-api/conversation/id/{conversation_id}` |

The official request needs to carry `Authorization: Bearer <access_token>`, `x-conduit-token`, `x-oai-turn-trace-id`, `x-oai-mcp-form-version: 2`, and the sentinel `OpenAI-Sentinel-Chat-Requirements-Token` (plus the corresponding token if PoW / Turnstile is triggered), all at the same time. The real web client also carries fingerprint headers such as `originator: Codex Browser`, `x-openai-web-frontend: codex_webview`, `x-openai-codex-window-type: browser`, `oai-did`, and `ChatGPT-Account-Id`.

For the full capture record, see [upstream-capture-2026-11-16.md](upstream-capture-2026-11-16.md).

## Basic Shapes

Common payload examples:

```text
"v1"
{"type":"resume_conversation_token",...}
{"p":"","o":"add","v":{...}}
{"v":{...}}
{"p":"/message/content/parts/0","o":"append","v":"..."}
{"type":"server_ste_metadata","metadata":{...}}
[DONE]
```

Handling recommendations:

| payload | Meaning | How to handle |
|:--|:--|:--|
| `"v1"` | Protocol version marker | Can be logged, usually does not affect business logic |
| `[DONE]` | The current SSE stream has ended | Stop reading further |
| JSON object | Event, message, or patch | Update conversation state by field |
| JSON string | Short text patch or protocol marker | Handle in context |
| Non-JSON content | Raw content | Keep as a raw event to avoid interrupting the stream |

## Common Fields

| Field | Description |
|:--|:--|
| `type` | Upstream event type, e.g. `resume_conversation_token`, `input_message`, `message_marker`, `title_generation`, `server_ste_metadata`, `message_stream_complete`, `user_signal_metadata`, `paragen` |
| `conversation_id` | The current conversation ID, obtainable from multiple events |
| `p` | Patch path, e.g. `/message/content/parts/0` |
| `o` | Patch operation, e.g. `add`, `append`, `replace`, `patch` |
| `v` | Patch value, which may be a string, an array, or may contain a full message |
| `c` | Message sequence number or cursor, common in `add`-type events |
| `message.id` | Message ID |
| `message.author.role` | Message role, commonly `system`, `user`, `assistant`, `tool` |
| `message.content.content_type` | Content type, e.g. `text`, `multimodal_text`, `model_editable_context` |
| `message.content.parts` | Content fragments, which may contain text, image pointers, or multimodal objects |
| `message.status` | Message status, e.g. `in_progress`, `finished_successfully` |
| `message.end_turn` | Whether the current turn ends |
| `metadata.tool_invoked` | Whether a tool was invoked this turn |
| `metadata.turn_use_case` | The purpose of this turn, e.g. `text`, `multimodal` |
| `metadata.generation` | Image-generation result metadata; only an image part where this is non-empty is treated as a generated image |
| `author.name` | The image tool name is `image_gen` or starts with `t2uay3k` |

## Conversation Start Event

The upstream usually first returns a resume token or a conversation token:

```json
{
  "type": "resume_conversation_token",
  "kind": "topic",
  "token": "...",
  "conversation_id": "..."
}
```

This event is mainly used to identify the conversation and restore context. The business layer usually only needs to save the `conversation_id`; the `token` should not be exposed to downstream users.

## Message add Scenario

A full message may appear via `add` or via an event carrying `v.message`:

```json
{
  "p": "",
  "o": "add",
  "v": {
    "message": {
      "author": {"role": "assistant"},
      "content": {"content_type": "text", "parts": [""]},
      "status": "in_progress"
    },
    "conversation_id": "..."
  },
  "c": 3
}
```

Such events are commonly used to create a new message. If the message role is `assistant`, subsequent text is usually appended via patches.

## Text Increment Scenario

Text output usually consists of multiple patches:

```json
{"p":"/message/content/parts/0","o":"append","v":"Hello"}
{"v":" world"}
{"p":"","o":"patch","v":[
  {"p":"/message/content/parts/0","o":"append","v":"!"},
  {"p":"/message/status","o":"replace","v":"finished_successfully"},
  {"p":"/message/end_turn","o":"replace","v":true}
]}
```

Handling notes:

| Shape | Meaning |
|:--|:--|
| `p == "/message/content/parts/0"` and `o == "append"` | Append content to the current text |
| `o == "replace"` | Replace the target field with the new value |
| `o == "patch"` and `v` is an array | A batch patch; handle in array order |
| Only `v` and `v` is a string | May be a text increment with the path omitted; handle together with the current text stream |

## Input Message Scenario

User input appears as `input_message` or an ordinary `user` message. An image-edit request includes the reference image uploaded by the user:

```json
{
  "type": "input_message",
  "input_message": {
    "author": {"role": "user"},
    "content": {
      "content_type": "multimodal_text",
      "parts": [
        {"asset_pointer": "sediment://file_input"},
        "编辑提示词"
      ]
    }
  },
  "conversation_id": "..."
}
```

Such a `sediment://...` represents an input attachment, not a generated result. Even though it can be downloaded, it must not be returned as an output image.

## Image Tool Success Scenario

When image generation or image editing succeeds, the upstream generally emits a tool message:

```json
{
  "v": {
    "message": {
      "author": {"role": "tool", "name": "t2uay3k.sj1i4kz", "metadata": {}},
      "content": {
        "content_type": "multimodal_text",
        "parts": [
          {
            "content_type": "image_asset_pointer",
            "asset_pointer": "sediment://file_result",
            "mime_type": "image/png",
            "size_bytes": 1041922,
            "width": 1254,
            "height": 1254,
            "metadata": {"generation": {"gen_id": "...", "width": 1254, "height": 1254}}
          }
        ]
      },
      "metadata": {"image_gen_title": "...", "model_slug": "gpt-6", "default_model_slug": "auto"},
      "channel": "commentary"
    }
  },
  "conversation_id": "..."
}
```

Only an image pointer that satisfies all of the following conditions should be treated as an output result:

| Condition | Description |
|:--|:--|
| `message.author.role == "tool"` | The source is a tool message |
| `message.author.name` is `image_gen` or starts with `t2uay3k` | The current image tool name |
| The image's `metadata.generation` is non-empty | Indicates a generated result rather than an input attachment |
| `asset_pointer` is `file-service://...` or `sediment://...` | Points to a resolvable image resource |

An image message may also appear in an asynchronous turn where `metadata.image_gen_async == true`; in that case you need to keep polling the conversation until the image appears.

## Image Pointer Types

| Pointer | Common source | Description |
|:--|:--|:--|
| `file-service://file_xxx` | Image tool output | Can be resolved via the file download endpoint |
| `sediment://file_xxx` | Input attachment or image tool output | The source must be determined together with the message role |
| `file_upload` | Placeholder during upload | Usually should not be treated as output |

Do not judge something as an output image just because `file_` or `sediment://` appears in the string. You must combine the message role and the task type.

## Policy Refusal Scenario

When the upstream refuses a request, it usually does not produce an image tool message but instead returns ordinary assistant text:

```text
I can't assist with that request. If you have another type of modification...
```

Common accompanying events:

```json
{"type":"title_generation","title":"Request Denied","conversation_id":"..."}
```

```json
{
  "type": "server_ste_metadata",
  "metadata": {
    "tool_invoked": false,
    "turn_use_case": "multimodal",
    "did_prompt_contain_image": true
  },
  "conversation_id": "..."
}
```

Handling notes:

| Condition | Behavior |
|:--|:--|
| There is assistant refusal text | Return the text message |
| `tool_invoked == false` | Indicates there is no actual tool result |
| No message with `role=tool` and a non-empty image `metadata.generation` | Do not collect output images |
| The user input message has an image pointer | Still treat it only as an input attachment |

## Moderation Scenario

Some requests may return a moderation event:

```json
{
  "type": "moderation",
  "moderation_response": {
    "blocked": true
  },
  "conversation_id": "..."
}
```

If `blocked == true`, the turn should be considered blocked by policy. If there is subsequent assistant text, that text should be returned first; if there is no text, an appropriate error message can be returned.

## marker and title Events

The upstream returns some auxiliary events:

```json
{"type":"message_marker","marker":"user_visible_token","event":"first"}
{"type":"message_marker","marker":"last_token","event":"last"}
{"type":"title_generation","title":"...","conversation_id":"..."}
```

These events are usually used for frontend display, title generation, or streaming status markers, and do not represent actual text content or image results.

## metadata Events

`server_ste_metadata` is used to describe this turn's scheduling and tool status:

```json
{
  "type": "server_ste_metadata",
  "metadata": {
    "tool_invoked": true,
    "turn_use_case": "multimodal",
    "model_slug": "i-mini-m",
    "did_prompt_contain_image": true
  }
}
```

Common determinations:

| Field | Description |
|:--|:--|
| `tool_invoked == true` | The upstream considers a tool was invoked this turn |
| `tool_invoked == false` | The upstream did not invoke a tool; common for refusals or plain-text responses |
| `turn_use_case == "text"` | Handle as a text response |
| `turn_use_case == "multimodal"` | A multimodal request; does not necessarily mean there is image output |
| `did_prompt_contain_image == true` | The input contains an image; does not mean the output contains an image |

## Determining the Result After the Stream Ends

After the SSE ends, you can determine the result in the following order:

1. If image tool output pointers have already been collected, resolve and download the output images.
2. If there is no output image pointer but there is assistant text, and this turn was blocked or no tool was invoked, return the text message.
3. If there is no output image pointer but there is a `conversation_id`, you can query the full conversation detail and keep looking for image tool output. A new conversation's detail may return only the `mapping` node tree with an empty `messages`, so you need to iterate over `mapping[*].message`.
4. When querying the full conversation, still only read messages where `author.role == "tool"` and the image `metadata.generation` is non-empty.
5. If there is neither an image result nor text, return an upstream error or an empty-result error.
