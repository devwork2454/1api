# Gitee mirror (GitHub → Gitee)

Primary host is **GitHub** (`devwork2454/1api`). Gitee is a **read mirror** for
China network fallback (source + optional release assets).

**Live mirror (this fork):** https://gitee.com/wbff/1api  
(`GITEE_OWNER=wbff`, `GITEE_REPO=1api`; Secrets already set on GitHub.)

## 1. Create the Gitee repository

1. Open https://gitee.com/ and sign in.
2. **新建仓库** → name `1api` (recommended, matches GitHub).
3. Leave it **empty** (no README / license / `.gitignore`).
4. Note your path: `https://gitee.com/<你的用户名>/1api`

## 2. Automatic sync (recommended: GitHub Actions)

On every push to `main` (and `v*` tags), workflow
[`.github/workflows/mirror-to-gitee.yml`](../.github/workflows/mirror-to-gitee.yml)
mirrors git history to Gitee.

In **GitHub** → repo **Settings → Secrets and variables → Actions**, add:

| Secret | Required | Example |
|--------|----------|---------|
| `GITEE_TOKEN` | yes | Gitee 私人令牌，勾选 `projects` 读写 |
| `GITEE_OWNER` | no | 默认用 GitHub 的 owner 名；Gitee 用户名不同时必填 |
| `GITEE_REPO` | no | 默认 `1api` |

Gitee 令牌：头像 → **设置** → **安全设置** → **私人令牌**。

推送后到 **Actions** 看 `Mirror to Gitee` 是否绿色。无 token 时 workflow 会跳过，不失败。

## 3. Optional: Gitee 官方「仓库镜像」Pull

若账号仍开放该功能：Gitee 仓库 → **管理** → **仓库镜像管理** → 添加 **Pull**
（GitHub → Gitee），选自动镜像。见：

https://help.gitee.com/repository/settings/sync-between-gitee-github

可与 Actions 二选一或并存；**只写 GitHub，Gitee 只读**，避免双向乱推。

## 4. Release 二进制（install.sh / `1api update`）

`scripts/install.sh` 与 `1api update` 顺序：

1. **Gitee** `wbff/1api`（默认，避免国内连 GitHub 15s 超时）
2. 失败则 **GitHub** `devwork2454/1api` 同 tag 附件

需要两边都有同名资产：

- `1api_{linux,darwin}_{amd64,arm64}.tar.gz`
- `checksums.txt`
- `install.sh`

打 tag `v*` 会触发 GitHub Actions GoReleaser；再用仓库脚本把资产同步到 Gitee 发行版（git mirror **不会**带附件）：

```sh
GITEE_TOKEN=... scripts/sync-gitee-release.sh v1.5.16-devwork1
```

## 5. 安装命令

```sh
# 推荐：直接 Gitee（需已知 VERSION，或已安装后用 1api update）
VERSION=v1.5.16-devwork1 sh -c \
  'curl -fsSL "https://gitee.com/wbff/1api/releases/download/${VERSION}/install.sh" | sh'

# 也可从 GitHub 拉 install.sh（脚本内会优先 Gitee 下二进制）
curl -fsSL https://github.com/devwork2454/1api/releases/latest/download/install.sh | sh

# 源码
git clone https://gitee.com/wbff/1api.git && cd 1api && make install

# 已安装：1api update 默认走 Gitee，失败再 GitHub
1api update
```

环境变量：`REPO`、`GITEE_REPO`（默认 `wbff/1api`）、`VERSION`、`CHARON_UPDATE_URL`（覆盖 `1api update` 入口 URL）。
