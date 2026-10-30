# macOS 构建、安装与发布

当前发布目标为 **macOS arm64**，安装包为 ZIP，MyGo 更新器使用框架生成的独立完整 `.tar.gz` 与签名 manifest。Intel 尚未完成真实运行验收，未加入发布矩阵。最低系统版本保留项目原有的 macOS 12.0；本机验证不能替代所有旧版 macOS 验收。

`VERSION` 是唯一权威版本源，首次发布版本为 `0.1.0`。`mygo.json` 只保留应用基础配置；`make build/package` 在忽略的临时目录派生 MyGo 配置，并将版本、提交、通道和执行环境注入客户端。请使用 Make 入口，不直接调用未派生版本的 `mygo build`。

## 依赖与日常命令

需要 macOS、Xcode Command Line Tools、`go.mod` 指定的 Go、Python 3.9+。界面测试使用 Node.js 22 和 npm 锁文件；应用运行不需要 Node。Dev 图标生成复用桌面 skill 的脚本，需要固定版本 Pillow：

```sh
python3 -m venv .venv
.venv/bin/python -m pip install -r scripts/requirements-dev.txt
npm ci --prefix tests/frontend
make check
make run
make build PYTHON=.venv/bin/python
make package PYTHON=.venv/bin/python
make install PYTHON=.venv/bin/python
```

- `make build`：默认 `dev` 通道，生成 `build/dev/darwin-arm64/gopad Dev.app`，不安装。
- `make package`：在同一目录生成 Dev ZIP，不发布。
- `make install`：先从源码构建并验签，再安装到 `~/Applications/gopad Dev.app`。`INSTALL_DIR="/绝对路径/gopad Dev.app"` 可用于隔离验收。Dev 安装不会替换正式应用、清配置或自动启动；目标正在运行时停止并要求先退出该副本。
- `make build CHANNEL=preview`：显式生成仅供本机 QA 的 ad-hoc Dev 包到 `build/preview/darwin-arm64/gopad Dev.app`。它不能通过安装入口，也不启用正式更新。
- `make build CHANNEL=release` / `make package CHANNEL=release`：要求干净的已提交源码、固定发行签名和更新密钥。正式包位于 `build/release/darwin-arm64/`。

Dev 使用独立 bundle ID `dev.knothhe.gopad.dev`、名称 `gopad Dev`、数据目录 `~/Library/Application Support/gopad Dev`、单实例身份以及右下角 Dev 标记；正式版使用 `dev.knothhe.gopad` 与 `gopad`。`build-info.json` 记录来源和确切源码提交；这些元数据不是密码学来源证明。

## 配置本机固定签名

Dev 构建默认要求长期复用的代码签名身份；缺失时明确失败，不偷偷降级 ad-hoc。配置命令只保存公开身份引用，不导出私钥：

```sh
make signing-list
make signing-config IDENTITY=这里填写所选身份的40位SHA1
make build PYTHON=.venv/bin/python
make install PYTHON=.venv/bin/python
```

选择保存在被 Git 忽略的 `.gopad-local.json`（身份 SHA-1 与证书 SHA-256）。`GOPAD_SIGNING_IDENTITY` 可作单次覆盖，不修改已记住的选择。失效、证书不匹配或签名失败会停止，保留原安装。

没有身份时，在“钥匙串访问”→“证书助理”→“创建证书”中建立长期使用的 `Self Signed Root` / `Code Signing` 身份，保留配对私钥，然后重新运行 `make signing-list` 选择。构建不会自动创建证书、修改信任根或上传私钥。固定自签名保持代码身份，不等于 Developer ID 或 Apple 公证，也不能保证下载后的 Gatekeeper 无提示。

## 发布前一次性配置

1. GitHub 公共仓库为 `knothhe/gopad`，`origin` 指向 `https://github.com/knothhe/gopad.git`，默认发布分支 `main`。向空仓库首次发布时，先推送初始提交建立远程 `main`，再运行首次发布入口。
2. 运行固定版本的 `go run github.com/egoist/mygo/cmd/mygo keygen`，按 MyGo 输出在仓库外保管私钥和离线备份。将已验证的 `owner/repository` 写入 `config/updates.json` 的 `github`，保持 `tagPrefix: "v"`、`deltas: 0`。CI 使用 `GITHUB_REPOSITORY`，若与已提交目标不一致会拒绝。
3. 运行 `make signing-upload` 进入交互引导，选择已有本机 Code Signing 身份，或提供仓库外含配对私钥的 P12。本机身份仅在明确选择后导出，系统可能要求授权读取私钥；脚本只重打包选中的证书及匹配私钥，随后清理临时材料。命令不修改本机 Dev 身份选择。
4. 使用下述 `make signing-upload` 一键配置三项仓库 Secrets：`MACOS_SIGNING_CERTIFICATE`（P12 的 Base64）、`MACOS_SIGNING_CERTIFICATE_PASSWORD`、`MYGO_UPDATER_PRIVATE_KEY`。命令会初始化缺少的 `config/macos-signing.json`（严格为 `schema: 1` 和证书 DER 的小写 SHA-256）及 `config/updates.json` 的 `publicKey`；已有公开值必须与传入材料一致。私钥/P12/口令不要放源码、普通 variables、缓存或 artifact。发行 job 缺任意必需材料即停止，不上传无签名更新。
5. 使用 Developer ID 并需要公证时，额外配置 `APPLE_ID`、`APPLE_TEAM_ID`、`APPLE_APP_PASSWORD`；必须三个同时存在。CI 在临时 keychain 存储 notary profile，MyGo 完成公证和 stapling。未提供时只有固定证书签名，**不能宣称已公证或通过 Gatekeeper**。
6. 将代码、公开配置和 workflow 纳入提交。发布工具要求工作区干净、存在 HEAD、分支和远程正确。

在 macOS 交互终端执行，需要 `gh` 已登录且有目标仓库 Secrets 权限，OpenSSL 支持 Ed25519：

```sh
# 展示当前状态，选择签名材料与更新私钥，校验后确认上传
make signing-upload

# 同样进入引导并校验，但不上传、不写公开配置
make signing-upload SIGNING_FLAGS=--dry-run

# 已备好 P12 时仍可显式指定路径；缺少的路径会由引导补齐
make signing-upload SIGNING_P12="/仓库外/certificate.p12" UPDATER_KEY="/仓库外/mygo-update.key"
```

引导先展示目标仓库、发行证书指纹、更新公钥、本机身份及三项 Secret 的存在性，再收集签名材料。可按编号选择本机身份，或输入已有 P12 路径；更新私钥可输入仓库外路径，已存在的 MyGo 默认密钥文件会作为路径提示。已有 P12 的密码隐藏输入，本机身份导出使用随机容器密码，密码不放在 Make 参数中。无可用身份时会给出“钥匙串访问”的创建/导入指引；无更新私钥时按前面的 MyGo keygen 命令生成并备份，再重新运行。不会自动生成或轮换身份。

目标仓库从 `origin` 解析，并与 `config/updates.json` 的 `github` 核对；可用 `RELEASE_REPO=knothhe/gopad` 显式指定，但仍须与两者一致。预检检查 P12 密码、唯一配对的代码签名身份、证书有效期和 Code Signing EKU、已有证书指纹，以及更新私钥和已有更新公钥的一致性。P12 只导入私有临时钥匙串，结束时恢复钥匙串搜索列表并清理临时材料；上传值经 stdin 传给 `gh secret set`，不回显。交互引导会在最终摘要后确认上传；输入 `q`、EOF 或 Ctrl-C 可取消，拒绝最终确认时不会写公开配置或上传 Secrets。完整指定文件参数时沿用直接校验、输入 P12 密码并上传的方式；无交互终端时立即给出使用说明。

所有预检通过后，命令先保存公开配置，再依次上传三项 Secrets。已有指纹或公钥不匹配时拒绝自动轮换；若本地缺少对应公开配置而远程已有 Secret，先恢复原公开配置再重试，避免覆盖未知身份。部分上传失败会停止并报告已成功的项，保留公开配置和已上传项，修复原因后可用同一份材料重试；GitHub 只能列出 Secret 名称，不能读回私钥或证明远程内容匹配。成功后检查并提交公开配置，再执行发布命令。

保留更新私钥是持续升级的前提；首次公开版本必须带有受控公钥。今后不能仅换 Feed 来更换旧客户端的信任根。

## 首次 v0.1.0 与后续升级

复用 SemVer skill 的 Python 发布引擎；首次与升级共用 `.release.json`，本地只负责检查、版本、annotated tag 与推送，唯一发布执行端是 GitHub Actions。

```sh
make release-initial                         # 只预览，不改版本、不构建、不推送
make release-initial RELEASE_FLAGS=--apply   # 当前 0.1.0 的首次 tag 与推送
make release VERSION_BUMP=patch              # 预览 0.1.1
make release VERSION_BUMP=patch RELEASE_FLAGS=--apply
make release VERSION_BUMP=0.2.0 RELEASE_FLAGS=--apply
```

首次入口不改写 `VERSION`、不创建版本提交；升级入口要求新版本更高，并创建版本提交。两者检查本地及远程 tag，GitHub 首次流程也检查可见 Release/草稿；网络或权限失败不会被当成“未发布”。预览中的 `release_executor` 应为 `github-actions`，本地 `build/assets` 应为空。需要 `gh` 已登录来做只读预检。

`--no-push` 只验证本地流程，不证明远程未发布。执行前请先检查预览；**本文命令是操作入口，不代表本次已经执行公开发布**。

## Tag CI 和制品

`.github/workflows/release.yml` 在 `v*` tag push 触发，也允许手动提供精确已有 tag 进行恢复。它检出该 tag 的完整历史、校验 `v + VERSION` 和 HEAD、执行检查，在 `macos-15` arm64 runner 构建。发布 job 单独拥有 `contents: write`；按 tag 串行，禁止并行覆盖。

构建 job 以临时 keychain 导入固定发行身份，验证证书 pin，完成后恢复原搜索列表并删除临时材料。应用使用成熟 MyGo 更新器和 MyGo CLI 产生的 Ed25519 签名更新格式，没有自行实现安装器或下载后执行脚本。

每个版本必须完整具有以下 **4 个**附件：

- `gopad-0.1.0-macos-arm64.zip`：手动安装的 app bundle。
- `gopad-0.1.0-darwin-arm64.tar.gz`：MyGo updater 的完整更新归档。
- `update-darwin-arm64.json`：该平台版本、固定版本 URL、大小和归档签名。
- `SHA256SUMS`：上述三个文件的校验值。

发布 job 再验归档签名、通道、版本、源码提交和确切附件集合，创建草稿，上传全部附件并下载校验后才公开。已有同名附件必须字节一致，绝不自动 `--clobber`；已有公开 Release 仅验证，不修改正文或资产。公开后再验证匿名下载路径。Release 正文由同一引擎生成 Full Changelog 和提交列表：

```sh
make release-notes RELEASE_REPO=knothhe/gopad RELEASE_TAG=v0.1.0
```

签名验证、版本身份与 manifest fixture 在本地可测试；没有真实 GitHub 运行和旧版升级验收前，只能称“流程配置与本地测试完成”，不能称“CI 发布和自动更新已验证”。

## 失败恢复

本地失败先保留现场并定位阶段。正确的本地 annotated tag 已存在且指向当前 HEAD 时，使用精确版本续推：

```sh
make release VERSION_BUMP=0.1.0 RELEASE_FLAGS="--resume --apply"
```

首次 tag 带有 `Release-Type: initial`。不移动 tag、不强推、不重复 bump。tag 已在远程但 CI 失败时，先确认旧 run 终止；优先重跑失败 job 或以相同精确 tag 手动触发 workflow。重复推送同一 tag 通常不会触发新构建。

同名且内容不同的资产会导致恢复停止，需要人工核查，不自动覆盖。若需要改源码，使用新的修复版本；已发布版本的信任、数据迁移和更新兼容性不能通过改 tag 绕过。

参考：[MyGo 更新协议](https://mygo.egoist.dev/docs/updates)、[GitHub runner 架构](https://docs.github.com/en/actions/reference/runners/github-hosted-runners)、[GitHub Release CLI](https://cli.github.com/manual/gh_release_create)。
