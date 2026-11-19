# Changelog

## 2.0.0 - 2026-11-16

+ [Refactor] The entire system was rewritten in Go, supporting high concurrency with an extremely low memory footprint.
+ [Refactor] The pages were completely rebuilt; chat, image generation, files, and voice were merged into the "Creation Studio".
+ [Update] The upstream protocol was updated to the latest web version, adding speech synthesis and transcription endpoints.
+ [Fix] Fixed an issue where the user key's last-used time was not updated.
+ [Note] Not compatible with historical data; after upgrading you must re-import accounts.

## 1.8.0 - 2026-07-28

+ [Added] Added configuration for the default upstream model name and default thinking effort, which can be changed on the settings page and overridden via the `-standard`, `-extended`, `-max` suffixes on the model name.
+ [Fix] Added a config to remove local conversations even when no image is produced, supporting asynchronous hiding of the corresponding conversation record when image generation fails, times out, or only returns text.
+ [Fix] `/v1/models` aggregates the official model list across account types, and text requests select accounts based on model permissions.
+ [Fix] Filter out hidden, non-final-channel assistant messages and those sent to internal tools, to prevent search instructions and reasoning content from leaking into the API output.
+ [Fix] Fixed an issue where output cleaning mistakenly removed spaces before punctuation in code and commands.
+ [Fix] Added a configurable hard timeout cap for the image-generation SSE stream to avoid upstream long connections hanging for a long time.
+ [Optimization] Database storage was changed to incremental sync, only adding, updating, or deleting records that changed, and preserving the IDs of unchanged records.

## 1.7.0 - 2026-07-05

+ [Removed] Removed the registration feature; the anti-abuse mechanism was causing GitHub accounts to be banned.

## 1.6.0 - 2026-07-04

+ [Fix] Fixed the sub2api import issue.
+ [Fix] Fixed frontend 404 and 405 issues.
+ [Added] Added a feature to delete the conversation record after an image is produced.
+ [Adjustment] Pro accounts are no longer treated as having unlimited quota — roughly 1000 images per day.

## 1.5.0 - 2026-06-13

+ [Added] Added a WARP / Privoxy / FlareSolverr unblocking solution; after hitting a Cloudflare block during registration, you can refresh the clearance and retry.
+ [Added] Added an `outlook_token` mailbox pool, supporting reading Outlook/Hotmail registration verification codes.
+ [Added] Added a web-search compatibility endpoint, image-edit mask support, and image-task-related capabilities.
+ [Optimization] Updated the sentinel/PoW acquisition method to improve upstream request compatibility.
+ [Optimization] Adjusted the proxy priority and the registration-request retry logic.

## 1.4.1 - 2026-06-03

+ [Added] Account refresh was changed to asynchronous mode, supporting frontend polling of refresh / re-login progress.
+ [Added] Added a re-login feature to the account-pool management page, supporting password login to recover abnormal accounts.
+ [Added] Automatically re-login abnormal accounts after a refresh (can be enabled on the settings page).
+ [Added] Image generation supports parallel mode; multiple images are generated simultaneously using independent threads and accounts.
+ [Added] On image polling timeout, automatically switch accounts and retry (up to 4 times); on connection timeout, retry on the same account with incrementally increasing waits.
+ [Added] The image double-confirmation mechanism and the "check first, then hit" behavior are configurable; when disabled, results can be returned directly without waiting.
+ [Added] Image task progress tracking, showing the current generation step (upload / warmup / fetching token / generating, etc.).
+ [Added] Continued polling after an image timeout; the frontend shows a "Keep waiting" button.
+ [Added] Added config items on the settings page for image double-confirmation, timeout wait time, auto re-login, etc.
+ [Optimization] Optimized scroll-loading performance on the image-generation page, with image lazy loading and saving/restoring the scroll position when switching sessions.

## 1.4.0 - 2026-05-31

+ [Added] Added reverse-engineering of AI-generated editable PSD files.
+ [Added] Added reverse-engineering of AI-generated editable PPT files.

## 1.3.1 - 2026-05-30

+ [Added] Added ChatGPT search debugging and Skills.

## 1.3.0 - 2026-05-30

+ [Added] Added reverse-engineering of the ChatGPT search interface.

## 1.2.4 - 2026-05-30

+ [Added] Added chat-completion caching and duplicate-request merging.
+ [Added] Added a one-click jump feature to the infinite canvas.

## 1.2.3 - 2026-05-29

+ [Added] Added account-level proxies.
+ [Fix] Fixed 503 error messages and a frontend email line-wrapping issue.

## 1.2.2 - 2026-05-29

+ [Added] Added Codex-route image generation, supporting 2k and 4k.
+ [Added] Added support for refreshing account info via RT.

## 1.2.0 - 2026-05-28

+ [Added] The current version baseline, including the web panel, drawing, account-pool management, registration bot, image management, log management, and settings capabilities.
+ [Added] The frontend version number can be clicked to view the version-update popup, showing the current version, the latest version, and the changelog.
+ [Optimization] Optimized the registration bot's efficiency, greatly increasing the success rate.
+ [Optimization] Optimized the configuration options on the image-generation page.
