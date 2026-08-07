# Gitee mirror (GitHub → Gitee)

Primary host is **GitHub** (`devwork2454/1api`). Gitee is a **read mirror** for
China network fallback (source + optional release assets).

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

## 4. Release 二进制（install.sh 回退）

`scripts/install.sh` 下载顺序：

1. **GitHub** `releases/latest`（或 `VERSION=`）
2. 失败则 **Gitee** 同名仓库的 release 附件

因此 Gitee 上除了 git 镜像外，若要让国内安装脚本也能下二进制，需要在
Gitee **发行版**里挂上与 GitHub 相同的：

- `1api_linux_amd64.tar.gz` 等
- `checksums.txt`
- `install.sh`（可选）

可用手动上传，或另加 workflow 在 GitHub Release 发布后同步附件（未内置）。

## 5. 安装命令

仍推荐：

```sh
curl -fsSL https://github.com/devwork2454/1api/releases/latest/download/install.sh | sh
```

脚本内部会先 GitHub 再 Gitee。环境变量：

```sh
REPO=devwork2454/1api
GITEE_REPO=<gitee用户>/1api   # 若 Gitee 路径不同
VERSION=v1.2.3
```
