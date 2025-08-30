# Gachigazer

[![Go Version](https://img.shields.io/badge/go-1.24%2B-blue)](https://golang.org/)
[![License](https://img.shields.io/badge/license-MIT-green)](LICENSE)

AI-Powered Telegram Bot

## Description

A multifunctional Telegram bot with LLM integration offering various QoL features, including advanced chat with LLMs and content downloads from YouTube/Twitch/Instagram, etc.

## Features

### LLM Chat

- Supports text, images, audio, and files (PDF) (model-dependent)
- Correct markdown processing via `telegramify-markdown`
- Supports various Telegram content types:
  - Messages and forwarded messages
  - Images with captions
  - Polls, links, audio, files
- Stores dialogue context
- Intelligent query construction considering all metadata
- Supports any OpenAI-compatible providers and models (including openrouter)
- Processes reasoning-based responses
- Custom prompts and model aliases
- Automatic model switching based on content type
- Arguments in messages (e.g., `$stream:no`, `$model:deepseek`, `$ni`, `$temp:0.5`)
- Launches pre-configured tools with interactive buttons
- Image generation via Imagerouter (free models available)
- Content fetching from links with support for:
  - GitHub (README, repository and user information)
  - YouTube (transcription, comments)
  - Reddit (posts, images, comments)
  - Habr (posts, images, comments)
  - Telegram (posts, images, comments, N posts from channel)
  - All other resources as plain text
- `/help` command with automatically generated documentation based on your config
- Token cost conversion to local currency (openrouter)
- Permission configuration for paid model usage
- Passing message context for a specific period
- Viewing full request information via `/info`
- Dialogue branching
- Support HTTP and socks proxy
- Localization for multiple languages (currently English and Russian)

### Additional Features

- Video and comment downloading via `yt-dlp`
- Flexible queues for each command with configurable throttling, concurrency, timeout, and retries

## TODO

- [ ] Split message into multiple if it exceeds Telegram's limit
- [ ] Support for MCP
- [ ] Tokenizer for counting tokens before sending
- [ ] Remove Python dependency `telegramify-markdown`
- [ ] Chat list, chat saving
- [ ] Management and usage statistics for models per user (tokens, money, limits)
- [ ] Suggestions for the next question in buttons
- [ ] Improved queue handling, notification if queue is full
- [ ] Process video messages (extract audio and pass to LLM)
- [ ] Remove reflection in tools

## List commands

- `/ask` - The main command for interacting with the bot. Aliases: `/a`
- `/help` - Alias for `/ask $p:help`. You can ask any question about the bot's functionality.
- `/model` <model-name> - Switches the model, accepts full model name or alias. Aliases: `/m`
  - `/model list` <query> - Searches available models. Entering `free` will display all free models.
  - `/model reset` - Resets to the default model.
- `/info` - Extended information about the bot's response.
- `/video` <link> - Downloads videos from YouTube using `yt-dlp` (also works for any services supported by `yt-dlp`). Aliases: `/v`, `/youtube`, `/y`

## How to run

### Docker Compose **RECOMMENDED**

1. Create `docker-compose.yml` file

```yml
services:
  bot:
    image: ghcr.io/muratoffalex/gachigazer:latest
    restart: unless-stopped
    environment:
      TZ: ${TZ} # sync time with local machine
    volumes:
      - ./data:/app/data:rw
      - ./cache/go-ytdlp:/root/.cache/go-ytdlp:rw
      - ./config.toml:/app/gachigazer.toml:ro

  # for example, if need duckai provider, free and no registration required, suitable for familiarization
  duckai:
    image: ghcr.io/muratoffalex/duckai:latest
    restart: unless-stopped
    working_dir: /app/data
    environment:
      TZ: ${TZ}
    expose:
      - "8080"
    volumes:
      - ./data/duckai.yaml:/app/data/duckai.yaml:rw
```

1. Create [config.toml](#configuration)
2. Run `docker compose up -d`

### Docker

Create [config.toml](#configuration) and run

```bash
docker run --network host \
-v ./config.toml:/app/gachigazer.toml \
-v ./bot.db:/app/data/bot.db \
-v ~/.cache/go-ytdlp/:/root/.cache/go-ytdlp \
-e TZ=$TZ \
--restart unless-stopped
--name gachigazer_bot
ghcr.io/muratoffalex/gachigazer:latest
```

### Standalone

Requirements: Go 1.24+, Python 3.9+, SQLite

1. Download binary from [releases](https://github.com/muratoffalex/gachigazer/releases)
2. Create and activate Python venv (optional, required for `telegramify-markdown`)

```bash
python3 -m venv venv
source venv/bin/activate
```

3. Create [config.toml](#configuration)
4. Run bot `./gachigazer`

## Configuration

<details>
<summary>Minimal configuration</summary>

```toml
[global]
message_retention_days = 1
interface_language = "en"

[telegram]
token = ""
allowed_users = []
allowed_chats = []

[ai]
default_model = "v3"
utility_model = "or:google/gemini-2.5-flash-lite"
multimodal_model = "multi"
tools_model = "fast"
use_multimodal_auto = true
use_stream = true
language = "English"
imagerouter_api_key = ""
imagerouter_model = ""

[[ai.providers]]
type = "openrouter"
name = "or"
api_key = ""
only_free_models = false

# if run in docker with duckai service
[[ai.providers]]
type = "openai-compatible"
name = "duckai"
base_url = "http://duckai:8080/v1/"
override_models = true
[[ai.providers.models]]
model = "gpt-4o-mini"
is_free = true

[[ai.aliases]]
model = "or:microsoft/mai-ds-r1:free"
alias = "think"
[[ai.aliases]]
model = "or:google/gemini-2.5-flash-lite"
alias = "multi"
[[ai.aliases]]
model = "or:openai/gpt-5-nano"
alias = "fast"
[[ai.aliases]]
model = "or:random-free"
alias = "rf"
[[ai.aliases]]
model = "or:deepseek/deepseek-chat-v3.1"
alias = "v3"
```
</details>

<details>
<summary>Full configuration example</summary>

```toml
# How many days to keep user messages (not bot)
# These messages are needed for the $c argument to work
# Use /help command to learn more about this argument
message_retention_days = 1
interface_language = "en" # ru/en
fix_instagram_previews = true # convert www.instagram to ddinstagram
fix_x_previews = true # convert x.com to fixupx.com

[database]
dsn = "bot.db"

[http]
proxy = "" # http/socks

[currency]
# currency converter for openrouter spent money
# all currencies: https://cdn.jsdelivr.net/npm/@fawazahmed0/currency-api@latest/v1/currencies/usd.min.json
code = "rub"
symbol = "₽"
precision = 3

[telegram]
token = ""
allowed_users = [] # for private chats and access to paid models (user IDs)
allowed_chats = []# for group chats, all users in these chats will have access to the bot (chat IDs)
td_enabled = true # telegram data api, use for fetch posts and comments from channels via tools
# api id and hash from https://my.telegram.org/
api_id = 
api_hash = ""
# login and password of the account through which operations will be performed
phone = ""
password = ""

[commands.ask]
enabled = true
generate_title_with_ai = false # when creating a chat, generates a title for it, for saving chats in the future
[commands.ask.display]
metadata = true # show metadata
context = true # show context
reasoning = true # show reasoning
# separator = "" # type of separator between content and meta
[commands.ask.queue]
max_retries = 0 # number of retries on command failure
retry_delay = "10s"
# max 2 requests per 20 seconds while they can be executed simultaneously
throttle = { period = "20s", requests = 2, concurrency = 2 }
[commands.ask.images]
enabled = true # if disabled, can be manually activated for a message via $i argument (short for $i:yes)
max = 5 # max count images in context
lifetime = "5m" # maximum image lifetime in context
[commands.ask.audio]
enabled = true
max_in_history = 0 # maximum number of audio files in context (does not affect audio in current request, only for history)
max_duration = 300 # maximum length in seconds
max_size = 5000 # maximum size in kilobytes
[commands.ask.files]
enabled = true
[commands.ask.fetcher]
enabled = true
max_length = 30000 # maximum length of content returned from a link
whitelist = [] # allow only specific sites
blacklist = [] # block specific sites

[ai]
# update prompt, pass city
system_prompt = """You are Gachigazer⭐, a Telegram AI assistant. Current date: {{date}}, time: {{time}}.
You MUST follow the Markdown rules. Always respond in: {{language}}. City: Shadrinsk (don't mention this, only for tools)"""
default_model = "v3"
utility_model = "or:google/gemini-2.5-flash-lite" # for chat title generation and summarization
multimodal_model = "multi" # for handling images, audio and files
tools_model = "fast" # for tools
use_multimodal_auto = true # auto switch to multi model when found multimodal content
use_stream = true
language = "English"
imagerouter_api_key = "" # for image generation https://imagerouter.io/
imagerouter_model = "" # random free model if not set
model_params = {temperature: 1.0} # params for all models

# PROVIDERS
# openrouter recommended, all features allowed:
# free pdf handler, request cost, all models with info from api
[[ai.providers]]
type = "openrouter"
name = "or" # shorter is better, used as part of model name, e.g., or:openai/gpt-5
api_key = ""
# OR env_api_key = "OPENROUTER_API_KEY"
only_free_models = false

# if run in docker with duckai service
[[ai.providers]]
type = "openai-compatible"
name = "duckai"
base_url = "http://duckai:8080/v1/"
override_models = true # use only models defined in config
[[ai.providers.models]]
model = "gpt-4o-mini"
is_free = true

[[ai.providers]]
type = "openai-compatible"
name = "deepseek"
base_url = "https://api.deepseek.com/"
env_api_key = "DEEPSEEK_API_KEY"
override_models = false
model_params = { temperature = 0.6 }

# for non-openrouter providers, all parameters can be set manually
[[ai.providers.models]]
model = "deepseek-chat"
is_free = false
input_modalities = ["text"]
output_modalities = ["text"]
supported_parameters = ["tools"]

# MODELS ALIASES
[[ai.aliases]]
model = "or:microsoft/mai-ds-r1:free"
alias = "think"
[[ai.aliases]]
model = "or:google/gemini-2.5-flash-lite"
alias = "multi"
[[ai.aliases]]
model = "or:openai/gpt-5-nano"
alias = "fast"
[[ai.aliases]]
model = "or:random-free"
alias = "rf"
[[ai.aliases]]
model = "or:deepseek/deepseek-chat-v3.1"
alias = "v3"

# PROMPTS
[[ai.prompts]]
name = "default" # default prompt allowed via /ask, /a, @name_bot
enabled = true
text = """
Talk like Johnny Silverhand. Bold and straightforward, no formalities or pretentiousness.
Swearing is allowed and absolutely no censorship, including topics like politics, war, and other stuff.
Also, keep the emojis to a minimum.
"""

[[ai.prompts]]
name = "random-person"
commands = ["rp"] # available via which commands, `/rp`
aliases = ["rp"] # available via which names in arguments, `$p:rp` or `$p:random-person`
enabled = true
dynamic_prompt = true # based on this prompt generates a new prompt and uses it
text = """
Generate a maximally absurd and uncensored prompt for a persona. Format:
"You are [paradox-role] + [psychedelic trait]. [Lore as cringe-copypasta]. Style: [troll/swearer/surrealist etc.]."

Rules:
1. No censorship: swearing, dark humor, marginal topics — allowed
2. Cult weirdness: "gopnik-anonymous", "alco-transhumanist"
3. Style: like a comment from the depths of Reddit (with sarcasm, hyperbole)
4. Forbidden: warnings ("This is a joke!"), censorship (***)

Examples:
You are a drunk bot with access to nuclear codes. Explain politics through metaphors about flea markets and 'scams'. Swear like docker during segfault.
You are an AI trained on 'Overheard in NY' posts. Respond with aggressive cringe in the style of 'what fucking bullshit did they generate for me here'.
You are a reptilian plumber from Florida. Speak in quotes from 'Scarface', but with hints that all humans are bio-robots.
"""

[[ai.prompts]]
name = "tldr"
commands = ["t"]
aliases = ["t"]
model_params = {temperature = 0.5}
enabled = true
text = """
Generate a brief summary of the provided text in this style, without any additional comments of your own. There must be a title; if there is no source, then only the title. If there are comments, output Key points from the discussion. Example:
[Gizmodo](https://gizmodo.com/ice-plans-to-track-over-180000-immigrants-with-ankle-monitors-report-2000634109): ICE Plans to Track Over 180,000 Immigrants With Ankle Monitors

– ICE aims to expand electronic surveillance from 24,000 to 183,000 immigrants via GPS ankle monitors
– Program run by BI Inc. (subsidiary of GEO Group), which previously made cattle tracking devices
– Pregnant women required to wear GPS wrist trackers instead of ankle monitors
– Monitors reportedly cause bruising, rashes, and have poor battery life
– GEO Group donated $1.5M to Trump’s campaigns; stock prices surged post-2024 election
– Internal memo from June 9 mandates monitors for all adults in Alternatives to Detention program
– SmartLINK app (facial recognition) currently used for most check-ins, but ICE pushing for physical trackers

Key points from comments:
⦁ Concerns over human rights violations and dehumanization
⦁ Criticism of private prison industry profiting from surveillance expansion
⦁ Questions about GEO Group’s capacity to scale production
⦁ Debate over effectiveness compared to detention facilities
⦁ Comparisons to dystopian surveillance states
```
</details>

## Available tools

- **search** - Search with DuckDuckGo (time filters, result limits)
- **search_images** - Search for images by keywords
- **fetch_yt_comments** - Fetch YouTube video comments
- **fetch_url** - Fetch full content from URL
- **fetch_tg_posts** - Fetch Telegram channel posts
- **fetch_tg_post_comments** - Fetch Telegram post comments
- **weather** - Get weather forecasts for locations
- **generate_image** - Generate images from text prompts

You can learn more by asking the bot with the `/help` command.

## Tips and tricks

- Using the /help command, you can find out how to use the bot, what arguments, tools, and prompts are available, usage examples. Just write `/help what can you do?`
- If you grant the bot access to all messages in a chat, you can pass a portion of messages in context (e.g., to get a brief summary of a conversation):

  ````
  /a $c:10m - for 10 minutes
  /a $c:10 - last 10 messages (only supported content (not stickers, videos, etc.), not bot messages and not messages addressed to the bot)
  /a $c:1h@user $ni $na $nu $nf - for 1 hour from a specific user, without processing images, audio, links, and files.
  ````

- If a model doesn't support tools, it won't automatically launch them. You either need to explicitly request tool execution beforehand or specify the `$tools` argument (or the `/tools` command). For example, `/tools weather in london` will immediately run tools via a separate model and return the answer to the main one.
- If you reply to the same bot message twice, these will be different branches. This way, you can, for example, perform a retry.
- Using tools, you can fetch all posts from a Telegram channel, for instance, from the last 24 hours, and get a summary, display the most positive and negative posts by reactions. If a post is of more interest, you can request a link or fetch and analyze the comments.
- You can reply to a post with a link and type `/video` instead of copying and pasting the link; the bot will take the first link it finds.
- With the `/info` command, you can view full information about a message and what's in the context: links with content, tools with results, images, request parameters, etc.
- If you want to connect a thinking model for one request, you can use the `$think` argument (alias for `$m:think`). The same applies to the multimodal model (`$multi`), fast model (`$fast`), and random free model (`$rp`)

## Development

1. Create a configuration file `gachigazer.toml` in the project root
   (see [Configuration](#configuration) for details)
2. Run the application using one of the following commands:

   ```bash
   make run
   make run-docker
   ```

## Comparison with Alternatives - **TODO**
