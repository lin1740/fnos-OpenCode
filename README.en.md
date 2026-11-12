# OpenCode (fnOS) — OpenCode for fnOS

[中文](README.md) | **English**

Run the open-source AI coding agent **OpenCode** on fnOS (FeiNiu NAS) with one click. Once installed it runs as a background service: open it from the fnOS desktop and let the AI read code, edit files, run commands and tests — all from the browser, no terminal required.

This repository is a **third-party, unofficial packaging** of [OpenCode](https://github.com/anomalyco/opencode) (by OpenCode contributors) for fnOS. It contains the FPK packaging scripts, the Go gateway, install/upgrade/uninstall scripts and adaptation notes (source tree: `opencode_all_<version>`).

> **Read before installing**: [2. Usage Notes](#2-usage-notes), section 1 "Known Limitations and Risks" — what this build **cannot do, does poorly, and may put at risk**. It matters more than the feature list.

---

## 1. Application Nature

1. This application (appname = `opencode`, display name "OpenCode") is a **third-party, unofficial packaging** of the open-source project OpenCode (<https://github.com/anomalyco/opencode>) for fnOS. It is packaged and distributed by a community developer, and has **no affiliation, authorization or partnership** with fnOS official (Guangzhou Tieran Zhizao Technology Co., Ltd.) or the upstream project team.
2. `opencode-linux-x64` / `opencode-linux-arm64` inside the package are the **official upstream binaries** (taken from the official npm platform packages). This project does **not modify, recompile or split their source code**; the fnOS adaptation is implemented entirely in the outer layer (custom scripts + Go gateway).
3. This application **only provides a runtime and an access path**. It bundles **no models, no API keys and collects no fees**. AI features require your own provider API key, or a local OpenAI-compatible endpoint such as Ollama / vLLM.
4. The application itself is **completely free, open source and non-commercial**. Fees charged by third-party model providers go directly to those providers and are unrelated to fnOS official, the upstream team or this packager.
5. One package supports both x86_64 and ARM64; the correct binary is selected at install time.
6. The service runs as a dedicated low-privilege system user `opencode-agent`, **never as root**; file access is limited to the project directory inside the application data area.

> The application is accessed through the fnOS unified gateway (opening it from App Center lands on `/app/opencode`, TLS terminated by the gateway). It **listens on no external TCP port** and enables no public tunneling.

---

## 2. Usage Notes

> Section 1 of this chapter lists the **known limitations, defects and risks of the current build**. Read it before installing, especially "Usage risks" and "no multi-user isolation"; the rest covers installation and usage.

### 2.1 Known Limitations and Risks

#### 2.1.1 Functional Limits

1. **No multi-user isolation**: the app runs as a **single instance**. Everyone who can log into your fnOS account shares the same instance, the same project directory and the same model configuration, and **session lists are visible to each other**. On a shared NAS, agree on usage or restrict access to trusted accounts only.
2. **Fixed project directory**: always `@appdata/opencode/projects` inside the application data area. The installer offers **no custom path**; moving it requires manual migration and is outside the supported scope.
3. **x86_64 / ARM64 only**: Linux binaries only — no 32-bit build, no Windows or macOS client form. Other architectures are rejected by the pre-install check.
4. **No bundled model, no bundled key, no network proxy**: you must supply a provider API key or a local endpoint. Without outbound internet, the online model catalog is empty and cloud models are unavailable.
5. **Single host, single instance**: no clustering and no way to move a session to another NAS and continue there.

#### 2.1.2 Known Defects

1. **Mobile is the weaker link**: 2.2.7 fixed the earlier "blank page / endless spinner" on phones (gateway-side path rewriting, clearing legacy Service Workers, a small mobile compatibility layer). Desktop Chrome / Edge / Firefox and mobile viewports have been verified, but **real-device coverage across HarmonyOS and the Android fnOS app's embedded WebView is not exhaustive**. Some models may still show misaligned UI, missing controls or a silent send button — please report with phone model and OS version.
2. **Stale browser cache can reproduce old problems**: after an upgrade, if the page is still blank or spinning, disable and re-enable the app in App Center, then clear the site cache in the browser — an old Service Worker may still be in control.
3. **Compatibility patches are coupled to the upstream page structure**: the gateway rewrites the entry prefix and injects a compatibility layer into the response stream, which depends on upstream frontend structure. **Large upstream frontend changes can cause misalignment, dead buttons or failed requests** until the package is re-adapted.
4. **Upgrades and restarts interrupt sessions**: upgrading or toggling the app restarts the service, aborting running AI tasks. Upgrade when idle.
5. **Low-spec devices struggle**: 1GB RAM is only the install threshold. Indexing large repositories and long sessions consume significant memory and CPU; low-end ARM units may lag, and in extreme cases the process can be reclaimed by the system.

#### 2.1.3 Usage Risks

1. **The AI really modifies your files**: files inside the project directory can be read, overwritten and deleted, with **no recycle bin**. Use Git or external backups for anything important.
2. **The AI executes code downloaded from the internet**: when you let it install dependencies or run scripts, it runs third-party code — a supply-chain risk. **Do not keep keys, certificates or identity documents in the project directory.**
3. **API keys are stored in plaintext on the NAS**: configuration lives in the application data directory (readable only by the runtime user), but a compromised NAS or stolen account means leaked keys. Set spending limits and rotate keys periodically.
4. **Code and prompts are sent to model providers**: with cloud models, project code and conversation content leave your device. Whether that is acceptable depends on your data and the provider's terms — use a local model for sensitive projects.
5. **Anyone who can log into fnOS can use this app**: there is no second authentication layer; the security boundary equals fnOS account security. Use strong passwords and 2FA if the NAS is exposed.

#### 2.1.4 Upstream and Version Risks

1. **Versions follow upstream with a delay**: there is lag between an upstream release and the App Center push. Wait for the update instead of replacing binaries yourself — a self-replaced binary is not supported.
2. **Upstream may change APIs and config formats**: model interfaces and config keys can change between versions; existing config may not be compatible and may need adjustment per upstream docs.
3. **Upstream licensing may change**: this app packages the current MIT-licensed upstream. If upstream changes its license, future builds will be re-evaluated.
4. **Third-party model providers are outside our control**: rate limits, bans, billing changes and endpoint shutdowns are decided by the providers, not by this app or its publisher.

### 2.2 Documentation

**See the upstream official docs: <https://opencode.ai/docs/>**

| Topic | Link |
| --- | --- |
| Official documentation (Intro) | <https://opencode.ai/docs/> |
| Model providers (75+) | <https://opencode.ai/docs/providers> |
| Configuration (`opencode.json`) | <https://opencode.ai/docs/config> |
| MCP servers | <https://opencode.ai/docs/mcp-servers> |
| Plugins and custom commands | <https://opencode.ai/docs/plugins> |
| CLI and TUI (not needed in this app) | <https://opencode.ai/docs/cli/providers> |

**Upstream project: <https://github.com/anomalyco/opencode>**
Upstream releases (version source): <https://github.com/anomalyco/opencode/releases>

Publisher website: <https://www.lin1740.de5.net/>

> The app opens the upstream Web UI, with the same features as upstream. The only differences are the access path (via the fnOS unified gateway) and a few browser-compatibility patches.

### 2.3 Installation

- **Option 1 (recommended)**: search "OpenCode" in fnOS App Center and install.
- **Option 2**: manually install `opencode_all_x.y.z.fpk` (App Center → Manual install). This repository contains the matching source tree, so you can build the package yourself.

Requirements:

1. fnOS ≥ 1.1.8 (`os_min_version`); architecture must be x86_64 or aarch64 (ARM64). Other architectures are rejected by the pre-install check.
2. RAM ≥ 1GB (enforced by the pre-install check); 2GB+ recommended for AI sessions and indexing.
3. Package size ~**235MB**; installed program files ~**512MB** (both architectures are included). Project files depend on what you put in.
4. Installation is fully offline — no image pulls, no dependency downloads. Usually **20–60 seconds**.
5. The installer wizard only has two informational pages ("Installation notes" and "Security notes"); there are **no port or path fields** — paths are fixed to avoid misconfiguration.

### 2.4 Access and Login

- **Single entry point**: click "OpenCode" on the fnOS desktop / App Center. You land on `/app/opencode` through the unified gateway. **Your fnOS login is the authentication** — there is no second password inside the app.
- Direct access via `http://<NAS_IP>:<port>` is not supported: the app only listens on a unix socket (`app.sock`) proxied by the fnOS gateway, and has **no external TCP port**.
- If the page is blank or keeps spinning: disable and re-enable the app in App Center once; if it persists, check `main.log` / `gateway.log` (see 2.5).

### 2.5 First-time Model Setup

1. Open the app → click "+" to create a session (add a project folder first).
2. Follow the UI to pick a provider and paste your API key, or choose a custom / local endpoint such as `http://127.0.0.1:11434/v1` (Ollama).
3. The model list comes from the upstream model catalog service (`models.dev`), so **the NAS needs outbound internet**. In a fully offline network, use a local endpoint instead.
4. The composer row shows agent / model / reasoning-effort selectors and the "+" button. Models that expose reasoning variants are sorted first for quick switching.

> A wrong API key will not crash the app; it only fails that session with an auth error. Fix the config and start a new session.

### 2.6 Data Locations

| Content | Location (admin view) |
| --- | --- |
| Project files (code and docs edited by you and the AI) | `File Manager → Admin view → All device files → Storage 1 (per your actual volume) → @appdata → opencode → projects` |
| Application data (config, API keys, sessions) | `@appdata → opencode → oc_home` under the same path |
| Logs (install / gateway / service / upgrade / uninstall) | `@appdata → opencode → *.log` (`install.log`, `main.log`, `gateway.log`, `serve.log`, `upgrade.log`, `uninstall.log`) |

- Viewing these paths **requires an administrator account** (regular members have no "Admin view" entry).
- On install, the project directory is created with a `我的项目` (My Projects) subfolder and a short usage note; it is owned by the app's runtime user so the AI can read and write it.
- On Windows you can open `Application files → opencode → projects` over SMB and edit the same files the AI works on.
- Program files are installed read-only; runtime writes go only to the application data directory and the project directory.

### 2.7 Ports and Network

| Item | Detail |
| --- | --- |
| External listening port | **None**. No TCP port is opened; the app cannot be reached by IP + port |
| Internal service port | `127.0.0.1:14096`, loopback only, used by the local gateway |
| Port registered in App Center | `4096` (registry / health-check value only, not an actual listener) |
| Access socket | `<app dir>/app.sock` (unix socket proxied by the fnOS gateway) |

- Outbound: your configured model provider APIs (e.g. `api.anthropic.com`, `api.openai.com`, `generativelanguage.googleapis.com`, `api.deepseek.com`, or your own endpoint) and the model catalog at `models.dev`.
- The upstream "remote / public tunnel" feature is **hard-disabled** in this app: `remote: false` is written at install time, `OPENCODE_DISABLE_REMOTE=1` is injected into the child process, and the config is re-checked before every start. The app will not expose itself to the internet.

### 2.8 Upgrade and Uninstall

**Upgrade**

- Update from App Center when a new version is available. Only program files are replaced and the service restarts; **config, API keys, sessions and project files are all preserved**.
- Running AI sessions are interrupted during the upgrade; upgrade while idle.
- Versioning follows upstream: upstream release → repackage → App Center push.

**Uninstall**

- The uninstall wizard has two choices, both defaulting to **keep**:
  - Application data (config / API keys / sessions): keep → nothing to reconfigure after reinstall; delete → wipes `@appdata/opencode`.
  - Project files: keep (recommended) → your code stays on disk; delete → only the project directory registered by this app is removed.
- Uninstalling without the wizard (e.g. from the CLI) **always keeps** data.
- Uninstall stops the gateway and `opencode serve`, cleans the socket and pid files, and revokes the runtime user's access grants (re-granted automatically on reinstall).

### 2.9 Permissions and Security Boundaries

1. **Non-root**: the service runs as `opencode-agent` and has no write access to system directories (`/etc`, `/usr`, fnOS system config).
2. **File scope**: by default only the project directory inside the application data area is readable/writable; other system directories and other users' data are not accessible.
3. **The AI really executes commands**: that is the product capability, not a vulnerability. Everything it runs stays inside the permission boundary above, but it **can delete or modify files in your own project**. Back up important data and do not point this app at sensitive directories.
4. **No root, no kernel modules, no Docker dependency**.
5. **Authentication is provided by fnOS**: the app performs no extra auth, so **anyone who can log into your fnOS account can use it**. On a shared NAS, manage accounts through fnOS users and permissions.

### 2.10 External Access

For access from outside, use fnOS official remote access / reverse tunneling, or your own VPN. Do **not** try to port-forward this app — it has no external port to forward. Do not expose the whole NAS to the internet without protection.

### 2.11 FAQ

| Symptom | Action |
| --- | --- |
| Blank page / endless spinner | Disable and re-enable the app in App Center; then check `gateway.log` / `serve.log` |
| Page loads but sending fails | Usually API key, endpoint URL or outbound network; verify with a local endpoint |
| Empty model list | The online catalog needs `models.dev`; use a custom / local endpoint in offline networks |
| Mobile (fnOS app) fails to load or misses controls | First verify the same entry in a mobile browser; if it still fails, report with phone model and OS version |
| Project files not found | Requires an administrator account; follow the path in 2.5 |
| Reinstall asks for API key again | "Delete application data" was selected during uninstall |
| Disk usage grows | Dependencies and build artifacts (`node_modules`, etc.) inside projects — clean up as needed |

### 2.12 Versioning Policy

- Versions **follow upstream official releases**, format `x.y.z`.
- If upstream publishes nothing new and only the packaging itself needs a re-release, use `x.y.z-n` (e.g. `2.2.6-1`).
- The bundled upstream binaries map one-to-one to upstream releases with no third-party modification; all fnOS adaptation lives in the outer layer (scripts + Go gateway).

---

## 3. Supporting the Publisher

If you find this software useful, you can support the publisher via the official website: <https://www.lin1740.de5.net/> — click "赞赏支持一下" (support) and scan the code. Any amount helps, and not donating is completely fine. Please put "赞赏" (tip) in the payment note, thank you.

A star on the project also helps.

---

## 4. Feedback

1. **Publisher email**: andforlin@foxmail.com
2. **Suggestion / feedback form**: <https://wj.qq.com/s2/28029808/2aab/>
3. **Issues with upstream OpenCode itself**: file them at <https://github.com/anomalyco/opencode/issues>

> The first two channels are for this application (FPK packaging, install/upgrade, gateway, access) and will get a reply. The third is for the upstream program and Web UI, handled by the upstream team.
>
> If you are not satisfied with how an issue was handled, or want to contribute: fork this repository, build with the official fnOS packaging tutorial, and submit through the first two channels. After auditing your code, the publisher may invite you to become a contributor. Thanks also to everyone who reports problems, gives suggestions or otherwise helps. See <https://www.lin1740.de5.net/>.

When reporting, please include: fnOS version, architecture (x86_64 / ARM64), app version, logs from the application data directory (`TRIM_PKGVAR`, default `/vol1/@appdata/opencode`: `install.log` / `main.log` / `gateway.log` / `serve.log`), and screenshots or a screen recording of the problem.

---

## 5. Open Source Notes

### 5.1 Upstream Project and License

The app runs the official upstream binaries unmodified, without recompiling or splitting their source.

- Project: <https://github.com/anomalyco/opencode>
- Maintained by: OpenCode contributors
- License: [MIT License](https://opensource.org/licenses/MIT) (full text in `app/LICENSE-MIT` inside the package)

Main direct upstream dependencies:

| Component | Author / Org | License | Project |
| --- | --- | --- | --- |
| Bun (embedded runtime) | Oven | MIT | <https://github.com/oven-sh/bun> |
| SolidJS (Web UI framework) | SolidJS | MIT | <https://github.com/solidjs/solid> |
| Vite (frontend build) | Vite | MIT | <https://github.com/vitejs/vite> |
| models.dev (model catalog) | anomalyco | see repo LICENSE | <https://github.com/anomalyco/models.dev> |

Transitive dependencies (provider SDKs, WebSocket/logging libraries, etc.) are compiled into the upstream binary; refer to the upstream repository for the complete list. This project does not bundle or distribute separate copies of them.

### 5.2 This Project (Packaging Scripts and Configuration)

- Project: <https://github.com/lin1740/fnos-OpenCode>
- Author: 键盘敲粥香 (lin1740)
- License: [Apache License 2.0](https://www.apache.org/licenses/LICENSE-2.0)
  > Apache License 2.0 permits free use, copying, modification, merging, publishing, distribution, sublicensing and sale, provided copyright notices, a copy of the license and any NOTICE file are retained and modifications are stated. The software is provided "as is", without warranty of any kind. The license includes an express patent grant.
- Contents: FPK manifest, install / upgrade / uninstall scripts, install wizard, Go gateway (unified gateway integration, path-prefix rewriting, static asset proxying, browser-compat patch injection, service supervision and self-healing), diagnostics and operational scripts — all developed by this project for fnOS adaptation.
- Note: the upstream binaries are **not modified**. The gateway only rewrites the entry prefix and injects a small compatibility layer in the response stream (e.g. pointing absolute asset paths back to the gateway prefix, clearing legacy Service Workers, adding a few polyfills needed by mobile browsers). The upstream UI and functional logic stay intact.

> **License note**: the bundled upstream program is MIT licensed and may be used, modified and redistributed with the original copyright and permission notices retained. API usage of each model provider is governed by that provider's terms.

### 5.3 Disclaimer

1. This is an **unofficial third-party application**, provided "AS IS", without any express or implied warranty (including merchantability, fitness for a particular purpose and non-infringement). Use at your own risk.
2. Use for learning and research only. Do not use it for any illegal purpose (including unauthorized access to other systems, evading risk controls, or distributing unlawful content). Users are responsible for complying with applicable laws and platform rules.
3. The publisher is not liable for any data loss, system failure or service interruption caused by installing or using this application. Back up your important data.
4. **AI execution risk**: OpenCode's core capability is letting the AI execute real shell commands and read/write files. Its output may not match your expectations; any resulting file modification, overwrite or deletion, or consequences of outbound requests, are your responsibility. Do not grant this app access to directories containing sensitive data.
5. Third-party components (OpenCode itself, provider SDKs) are maintained by their authors; their functionality, stability, security and compliance are outside this project's control, and upstream updates may change behavior.
6. This application does not actively collect personal data; runtime data stays on your device. Note that when using AI features, your input and project code are sent to the model provider you configured, subject to that provider's privacy policy.
7. **Model costs**: fees for third-party model services are charged by the providers and are unrelated to fnOS official, the upstream team or this packager.
8. **Commercial use**: this application is Apache-2.0 licensed and may be used commercially; however, before any commercial deployment (offering AI coding as a service, preinstalling and selling, etc.), verify the commercial terms of the model providers you connect.

### 5.4 Other Notes

1. **Trademarks**: OpenCode, Claude, GPT, Gemini, DeepSeek, Ollama, Bun, SolidJS, Vite and related marks belong to their respective owners. This project uses these names only for technical description, claims no trademark rights, and implies no official partnership or endorsement.
2. **Endorsement**: integration with upstream OpenCode is technical compatibility only and does not imply endorsement by the upstream team.
3. **Upstream changes**: upstream may change features, APIs or licensing at any time. This project will try to keep up but is not responsible for incompatibilities or licensing changes caused by upstream; refer to the upstream announcements.
4. **Component integrity**: upstream binaries come from official release channels and are not modified. If you replace or modify them yourself, you are responsible for any malfunction or compliance issue.
5. **Omissions and corrections**: upstream dependencies are complex; some transitive dependency license information or links may be incomplete or inaccurate. If you spot a missing project, a wrong license or a broken link, please tell us via the feedback channels in section 4 and we will correct it.
6. **Source availability**: the packaging scripts and configuration are released under Apache License 2.0 and published in this repository (organized as `opencode_all_<version>` directories) as versions are released; refer to the actual repository contents.

---

## Acknowledgements

- [OpenCode](https://github.com/anomalyco/opencode) — the AI coding agent and Web UI
- [Bun](https://github.com/oven-sh/bun), [SolidJS](https://github.com/solidjs/solid), [Vite](https://github.com/vitejs/vite) — upstream runtime and frontend stack
- The fnOS developer community
