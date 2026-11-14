# Git Pack 热更新方案

> 状态：设计中，分支 `feat/git-pack-update`

## 1. 背景

NIKKEAutoScript、ZenlessZoneZero-OneDragon 这类 Python 项目分两层更新：启动器和运行时随发版分发，源码则用 git 跟着分支更新。

现有的 zip 增量包会直接改工作区，HEAD 却不动，git 会把这些改动看成一堆本地修改：

- ZZZ 判定为 `LOCAL_CHANGES`，停止自动更新；
- NIKKE 下次启动照样去上游 fetch，再 `reset --hard`，Mirror酱 那次下载等于白费。

本方案让 Mirror酱 直接下发 git 对象，客户端更新后的仓库状态和真正执行 fetch 一致。下发的文件内容不可变，可以被 CDN 缓存，并复用现有"按版本对生成增量 + 签名 CDN URL"的流程。

## 2. 两层模型

| | 发版（打 tag） | 热更新（git） |
|---|---|---|
| 更新的内容 | 启动器 / 运行时 / 全量包 | `src` 源码 |
| 触发方式 | 推 `v*` tag，走项目现有的发版流程 | 推到跟踪的分支，走新增的工作流 |
| Mirror酱 资源 | 现有的 zip 资源 | 新建的 `pack` 资源 |
| 版本名 | semver，如 `v2.4.6` | 提交时间，如 `20261008153000` |
| 频率和大小 | 少、包大 | 多、包小 |

两个资源的版本互不影响。同一个资源里不会混用两种版本名，所以切换 channel 时比较版本（`doCompare`）不会出问题。

## 3. 不采用的做法

| 做法 | 不采用的原因 |
|---|---|
| 自建 smart HTTP 镜像 | 协商是 POST 请求，响应按每个客户端的状态现生成，CDN 无法缓存，全部流量和发版峰值都压在源站。客户端用 `git fetch` 轮询还会产生大量协商请求，每次都要校验 CDK |
| dumb HTTP | libgit2 不支持，ZZZ 用不了；不支持浅克隆，NIKKE 的 `--depth=20` 会失败；只能按整个 pack 下载 |
| bundle-uri | 只有 git 命令行支持，客户端要额外配置；静态的 bundle 列表很难接入 CDK 校验 |
| CI 上传 `git bundle` | 完整历史太大，而且每个版本都要重复上传；增量 bundle 需要 CI 知道上一次发布的 commit |
| thin pack | 代码文件本身不大，省不了多少；pygit2 无法补全 thin pack；服务端生成时需要旧版本的文件内容，而旧的全量包会被定时清理 |

## 4. 约定

### 4.1 资源

- 每个项目单独建一个资源，更新类型为 `pack`，os 和 arch 留空，因为源码不分平台。
- 这个资源和项目已有的发版 zip 资源分开，因为一个资源只能有一种更新类型。

### 4.2 版本名

版本名用 committer 时间，按提交本身记录的时区，格式为 `20060102150405`。`internal/pkg/vercomp` 的 `DateTimeParser` 已支持这种格式。

客户端只有 HEAD，要靠它自己算出当前版本名，才能调用 `/latest`，所以 CI 和客户端必须用同一个规则计算：

- git：`git log -1 --format=%cd --date=format:%Y%m%d%H%M%S <commit>`
- pygit2：`datetime.fromtimestamp(c.commit_time, timezone(timedelta(minutes=c.commit_time_offset))).strftime('%Y%m%d%H%M%S')`

限制：同一秒内有两个提交时版本名会重复，第二个上传会被拒绝，CI 会报错。

### 4.3 channel

channel 和分支的对应关系在 CI 里配置，比如 master 或 main 对应 stable，dev 或 test 对应 beta。服务端不需要知道分支。

## 5. CI 上传

每次推送到跟踪的分支，就上传这个 commit 的快照：commit 本身加上它完整的树和文件，不带历史。浅克隆就够了。

```yaml
on:
  push:
    branches: [master, dev]
jobs:
  upload:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4          # depth=1 即可
      - run: |
          echo "NAME=$(git log -1 --format=%cd --date=format:%Y%m%d%H%M%S)" >> "$GITHUB_ENV"
          git rev-list --objects --no-walk HEAD | git pack-objects --stdout > snapshot.pack
      # 走现有上传流程：POST /resources/:rid/versions 拿到预签名地址，
      # 上传后调用 POST /resources/:rid/versions/callback，带上 NAME 和 channel。
      # 也可以用 MirrorChyan/uploading-action，它需要支持 pack 类型。
```

包里恰好只有一个 commit，服务端从包里就能取出它的 SHA，CI 不需要另外上传。

## 6. 服务端

### 6.1 下发的包格式

全量包和增量包都是同一种 zip：

```
pack-<sha>.pack
pack-<sha>.idx
meta.json        {"commit": "<目标 commit>", "base": "<客户端当前 commit，全量包为空>"}
```

pack 不是 thin pack，每个对象的差量基础都在包里，`.idx` 由服务端生成。客户端把这两个文件放进 `.git/objects/pack/` 就能用。

### 6.2 导入（处理存储的任务）

上传回调触发处理存储的任务，在本地临时目录中执行以下步骤：

1. 上传校验：`doVerifyRequiredFileType` 新增对 pack 文件的识别，文件后缀为 `.pack`，文件头为 `PACK`。
2. 在临时 bare 仓库里执行 `git index-pack`，校验包的完整性并生成 `.idx`。
3. 确认包里恰好有 1 个 commit，记为 C；`git rev-list --objects --no-walk <C>` 必须能完整遍历，即树和文件都齐全。
4. 把 `git ls-tree -r -t <C>` 的结果，加上根树，写进这个版本的 `file_hashes`，格式为"路径 → 对象 ID"。同一列在 zip 资源里存的是文件的 sha256，按资源类型区分含义。
5. 把 `pack`、`idx` 和 `meta.json`（`base` 为空）打成 zip，复制到 ossfs，作为这个版本的全量包。

### 6.3 生成增量（生成增量的任务）

每个版本对的任务都不保留本地状态，哪个实例都能执行，不需要常驻的本地仓库，也不需要专门的队列。

1. 在本地临时目录 `git init --bare`，把目标版本 C 的全量包里的 `pack` 和 `idx` 放进 `objects/pack/`。
2. 需要发送的对象 = `git rev-list --objects --no-walk C`，再去掉客户端当前版本 A 的 `file_hashes` 里已有的对象 ID。
3. 用 `git pack-objects <临时目录>/out` 生成 `.pack` 和 `.idx`，打成 zip，`meta.json` 写 `{"commit": C, "base": A}`。
4. 增量包达到全量包的 80% 时，不保留，改发全量包。规则沿用 `patchTooLarge`。
5. 复制到 ossfs，写入 Storage，然后删除临时目录。

这一步复用了 `9acbb4b` 引入的三项机制：

- 增量 Storage 上的唯一索引；
- 文件名带内容哈希，形如 `<current>-<哈希前缀>`，不同次生成的文件不会互相覆盖；
- 任务重复执行时，发现已有记录就提前返回。

### 6.4 跨版本

- 用户在 A、最新版本是 E 时，直接生成 A→E 这一对的增量包，中间版本不参与，隔几个版本都一样。
- 生成时只读取目标版本 E 的快照（最新版本不会被清理），以及 A 在数据库里的 `file_hashes`，不需要 A 的快照文件。
  - 定时清理（`internal/repo/raw.go`，每天 05:00）对每个 channel、os、arch 只保留最近 2 个全量包，更早版本的快照文件早已删除。
- 因此，只要 A 的数据库记录和它的 `file_hashes` 还在，就能生成增量。能往回跨多远，取决于 `file_hashes` 保留多久。
- 跨得越远，增量包越大。达到全量的 80% 时改发全量快照，客户端的应用步骤不变。

### 6.5 `/latest`

- 请求方式不变：`GET /resources/:rid/latest?current_version=&channel=&cdk=`。
- 响应只有一处变化：全量包和增量包都返回 `update_type: "pack"`，不新增字段。目标 commit 写在包里的 `meta.json` 中。
- 增量包还没生成时，和 zip 资源一样先返回全量快照，同时投递生成任务。

### 6.6 运行环境

`Dockerfile` 需要安装 git：`apk add --no-cache git`。

## 7. 客户端接入

### 7.1 通用步骤

1. 用 4.2 的规则从 HEAD 算出当前版本名，请求 `/latest`。还没有仓库时，不传当前版本。
2. 下载 zip，校验 sha256，解压到临时目录。
3. `meta.base` 不为空、但和当前 HEAD 不一致时，改成不带当前版本重新请求，拿全量包。
4. 把 `pack` 和 `idx` 放进 `.git/objects/pack/`；还没有仓库时先 `git init`。
5. 把 `meta.commit` 追加进 `.git/shallow`，把它记为浅克隆边界，本地历史会从当前版本直接跳到目标版本。
6. 把分支指向 `meta.commit`，然后 checkout。

任何一步失败，就退回项目原来的 git 代码源。

### 7.2 NIKKEAutoScript（调用 git.exe）

- **配置**：在 `deploy.yaml` 中新增 CDK 和更新源开关。
- **启动时**（`deploy/starter.py` 中 `Starter.prepare` 调用 `git_update` 的位置）：
  - 按 7.1 的步骤执行；
  - 第 6 步用 `git update-ref refs/remotes/origin/<Branch> <commit>`，然后接上它现有的 `git reset --hard origin/<Branch>` 和 `pip_install`；
  - 首次安装时，`git init` 之后再执行 `git remote add origin <Repository>`，保证以后退回 GitHub 也能用。
- **运行中的定时检查**（`module/webui/updater.py` 的 `_do_check_update`）：改为调用 `/latest`，不再执行 `git fetch`。代码里残留的 `GitOverCdn` 分支（`goc_client.get_status()`）就是放这段逻辑的位置。
- **更新提示**：改用 `/latest` 返回的 release note。跳到浅克隆边界之后，`git log before..after` 只能列出 1 个 commit。

### 7.3 ZenlessZoneZero-OneDragon（使用 pygit2）

- **接入点**：在 `repository.yml` 中把 Mirror酱 加成一个"不是 git 地址"的代码源，CDK 放在 `env_config` 里。`GitService._fetch_remote` 遍历候选代码源时，轮到 Mirror酱 就执行以下步骤：
  1. 按 7.1 的第 1 到 3 步下载并校验。
  2. 在 `.install/git_fetch_tmp/` 下建一个临时 bare 仓库，和它现有的 fetch 临时仓库做法相同：
     - `pack` 和 `idx` 放进 `objects/pack/`；
     - `objects/info/alternates` 指向正式仓库的对象目录，这样当前版本的对象在临时仓库里也能看到；
     - 建 `refs/heads/<branch>`，指向 `meta.commit`；
     - `shallow` 文件 = 正式仓库现有的 shallow 内容，再加上 `meta.commit`。
  3. 交给现有的 `_import_fetch_result`。它会通过本地路径导入对象，同步 shallow，失败时回滚。
- **之后的流程不变**：模块清单兼容性检查（不兼容时返回 `RUNTIME_INCOMPATIBLE`）、checkout 和 `uv sync` 都照旧。
- **失败处理**：Mirror酱 源失败，就按代码源不可用处理，继续尝试 GitHub、CNB、Gitee。
- **例外**：首次运行要拉取和集成运行时匹配的内置 tag，`/latest` 给不了指定版本，所以首次运行仍走 git 代码源。

## 8. 与发版的配合

- **热更新能不能装，由客户端自己判断**：
  - ZZZ 用快照里的 `module_manifest.py` 和当前 `.runtime` 对比，不兼容就继续使用当前代码，等用户先通过发版资源更新启动器。
  - NIKKE 启动时按 `requirements.txt` 的差异补装依赖。
- **更新启动器后，热更新能自然接上**：tag 打在跟踪分支上的 commit 时，这个 commit 已经作为 `pack` 版本上传过，客户端可以直接拿到之后的增量包。tag 打在其他 commit 上时，服务端查不到这个版本，会回退为全量快照。

## 9. 成本

- 所有文件内容不可变、URL 不变，CDN 可以缓存。热更新包小、请求集中，命中率高；回源的只是少数几个热点文件。
- 检查更新走 `/latest` 这个 JSON 接口，有缓存，不会对 git 服务做协商请求。
- 增量包是非 thin pack，改过的文件整个压缩后发送，体积和现有的 zip 增量包相当。

## 10. 服务端改动清单

| 位置 | 改动 |
|---|---|
| `internal/model/types`、Resource | 新增 `pack` 更新类型 |
| `doVerifyRequiredFileType` | 识别 pack 文件 |
| 处理存储的任务 | pack 类型按 6.2 导入，生成全量包 |
| 生成增量的任务 | pack 类型按 6.3 生成；新建 `internal/pkg/gitpack` 封装 git 命令 |
| `doProcessUpdateRequest` | pack 资源返回 `update_type: pack` |
| `Dockerfile` | 安装 git |
| 接入文档或脚本 | CI 上传步骤、客户端应用步骤 |

## 11. 第一版不做、待定事项

- 包还没生成时返回 `pending` 和 `retry_after`。第一版先回退为全量快照。
- 发版时按用户数，给用户最多的几个旧版本提前生成增量包。这需要按版本统计用户数；`collect` 目前收到了 `currentVersion`，但没有存下来。
- `MirrorChyan/uploading-action` 支持上传 pack 文件。
- 不支持 git 子模块：子模块的对象不在快照里。
- 最终失败时写入带原因的跳过标记，并释放去重键。这件事和 zip 增量共用，另行处理。
