# 发布流程

本文档记录 `@hongchengxianyue/echo-dust-code` 的完整发布顺序。npm 包的 `postinstall` 会从 GitHub Release 下载二进制资产，所以必须先发布 GitHub Release，再发布 npm，不能只 `npm publish`。

## 发布前检查

1. 确认工作区状态：

```bash
git status --short --branch
```

2. 更新 `package.json` 的 `version`，例如从 `0.1.4` 改到 `0.1.5`。

3. 跑本地校验：

```bash
go test ./...
go vet ./...
npm run check
```

4. 提交并推送。提交信息必须使用中文：

```bash
git add <changed-files>
git commit -m "发布 0.1.5"
git push origin master
```

## 创建 GitHub Release

设定版本变量：

```bash
VERSION=0.1.5
TAG=v0.1.5
OUT=/tmp/echo-dust-code-$TAG
mkdir -p "$OUT"
```

构建全部 npm postinstall 需要的资产：

```bash
scripts/build-release-artifacts.sh linux amd64 "$OUT"
scripts/build-release-artifacts.sh linux arm64 "$OUT"
scripts/build-release-artifacts.sh darwin amd64 "$OUT"
scripts/build-release-artifacts.sh darwin arm64 "$OUT"
scripts/build-release-artifacts.sh windows amd64 "$OUT"
scripts/build-release-artifacts.sh windows arm64 "$OUT"
```

必须生成以下 6 个文件：

```text
echo-dust-code-linux-amd64.tar.gz
echo-dust-code-linux-arm64.tar.gz
echo-dust-code-darwin-amd64.tar.gz
echo-dust-code-darwin-arm64.tar.gz
echo-dust-code-windows-amd64.tar.gz
echo-dust-code-windows-arm64.tar.gz
```

创建 tag 和 GitHub Release：

```bash
git tag "$TAG"
git push origin "$TAG"
gh release create "$TAG" "$OUT"/*.tar.gz --title "$TAG" --notes "Release $TAG"
```

验证 release 和 Linux x64 下载链接：

```bash
gh release view "$TAG" --json tagName,assets
curl -I "https://github.com/HongChengXianYue/EchoDustAgent/releases/download/$TAG/echo-dust-code-linux-amd64.tar.gz"
```

`curl -I` 必须返回可下载响应，不能是 `404`。如果这里是 `404`，npm 安装必然失败。

## 发布 npm

确认 npm 登录状态：

```bash
npm whoami --cache /tmp/npm-cache
npm view @hongchengxianyue/echo-dust-code version --cache /tmp/npm-cache
```

如果 `npm whoami` 返回 `401 Unauthorized`，先登录或配置有发布权限的 token：

```bash
npm login
```

发布 npm：

```bash
npm publish --access public --cache /tmp/npm-cache
```

发布后确认 npm latest：

```bash
npm view @hongchengxianyue/echo-dust-code version --cache /tmp/npm-cache
```

## 安装验证

发布完成后，在一个干净环境执行：

```bash
npm install -g @hongchengxianyue/echo-dust-code@latest
echo-dust-code --help
```

如果安装时报：

```text
postinstall failed: download failed: 404 Not Found
```

优先检查对应版本的 GitHub Release 是否存在，以及 release 中是否上传了当前平台需要的 tarball。

## 常见失败

- `npm publish` 成功前没有 GitHub Release：用户安装时会下载 `v<package.json version>` 下的 tarball 并 404。
- `npm whoami` 返回 `401 Unauthorized`：当前机器没有有效 npm 登录态，不能发布 npm。
- `npm publish` 返回 `404 ... no permission`：通常是当前 npm 账号没有该 scope/package 的发布权限。
- 构建 release 资产时下载 gopls 失败：需要允许 `scripts/build-release-artifacts.sh` 访问 Go proxy。
- npm 因 `~/.npm/_logs` 权限失败：使用 `--cache /tmp/npm-cache`。
