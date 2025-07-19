-----

# SSH 和 Git 密钥切换工具 (GSS)

GSS (Git/SSH Switcher) 是一个用 Go 语言编写的命令行工具，旨在简化多 SSH 密钥对及其相应 Git 配置的管理。它能让你生成、导入、列出、切换和删除 SSH 密钥对，并自动更新你的 `~/.ssh/config` 和 Git 设置（`user.name`、`user.email`、`core.sshCommand`），从而实现不同身份之间的无缝切换。

-----

## ✨ 功能特性

  * **生成 SSH 密钥对**：快速创建新的 RSA SSH 密钥对。
  * **导入现有密钥**：将你现有的 SSH 私钥和公钥添加到 GSS 进行管理。
  * **列出密钥对**：查看所有由 GSS 管理的 SSH 密钥对的详细信息，包括当前激活状态。
  * **切换密钥对**：通过更新 `~/.ssh/config` 轻松切换活跃的 SSH 密钥。这还会更新你的 Git 用户名、邮箱和 SSH 命令，以匹配所选密钥的配置。支持全局和本地 Git 范围。
  * **删除密钥对条目**：从 GSS 配置中删除 SSH 密钥对条目。（注意：这**不会**删除实际的密钥文件。）
  * **持久化配置**：GSS 将你的密钥对配置存储在 `~/.gss/config.json` 中，确保你的设置在会话之间保持不变。

-----

## 🚀 快速开始

### 前提条件

  * Go (Golang) 环境（用于从源代码构建）
  * 系统上已安装 Git

### 安装

要安装 GSS，你可以从源代码构建它：

1.  **克隆仓库**：

    ```bash
    git clone https://github.com:winnerxue/gss.git
    cd gss
    ```

    （请将 `https://github.com:winnerxue/gss.git` 替换为你的实际仓库链接。）

2.  **构建可执行文件**：

    ```bash
    go build -o gss .
    ```

3.  **将可执行文件移动到你的 PATH 中**：

    ```bash
    sudo mv gss /usr/local/bin/
    ```

    这会使 `gss` 命令在全局范围内可用。

-----

## 📖 使用方法

GSS 使用基于子命令的接口。

```bash
gss <command> [options]
```

### 命令

  * `generate`, `gen`：生成新的 SSH 密钥对。
  * `import`, `i`：导入现有的 SSH 密钥对。
  * `list`, `ls`：列出所有管理的 SSH 密钥对。
  * `switch`, `s`：切换到指定的 SSH 密钥对。
  * `delete`, `del`：从配置中删除 SSH 密钥对条目。

-----

### 命令详情

#### `gss generate | gen -g <name>`

生成一个具有指定名称的新的 RSA SSH 密钥对。密钥将存储在 `~/.gss/` 中。

  * `-g <name>`：**必填**。新密钥对的名称（例如，`work_key`、`personal_key`）。

**示例：**

```bash
gss gen -g my_new_key
```

#### `gss import | i -i <private_key_path> -p <public_key_path> -n <name> -git-email <email> -git-name <git_user_name> [-c <ssh_config_path>]`

将现有的 SSH 密钥对导入 GSS。你必须提供私钥和公钥的路径、条目的名称以及相关的 Git 用户详细信息。

  * `-i <private_key_path>`：**必填**。你现有私钥的路径。
  * `-p <public_key_path>`：**必填**。你现有公钥的路径。
  * `-n <name>`：**必填**。用于在 GSS 中识别此密钥对的唯一名称（例如，`github_personal`）。
  * `-git-email <email>`：**必填**。与此密钥关联的 Git 用户邮箱。
  * `-git-name <git_user_name>`：**必填**。与此密钥关联的 Git 用户名。
  * `-c <ssh_config_path>`：**可选**。特定于此密钥的额外 SSH 配置文件路径（例如，`~/.ssh/config_github`）。当此密钥激活时，此文件的内容将被附加到你的主 `~/.ssh/config` 中。

**示例：**

```bash
gss import -i ~/.ssh/id_rsa_github -p ~/.ssh/id_rsa_github.pub -n github_work -git-email "work@example.com" -git-name "Work User" -c ~/.ssh/config_github
```

#### `gss list | ls`

列出所有由 GSS 管理的 SSH 密钥对，包括它们的关联路径和当前是否活跃。

**示例：**

```bash
gss ls
```

#### `gss switch | s [-i <index>] [-s <scope>]`

切换活跃的 SSH 密钥对。

  * `-i <index>`：**可选**。要切换到的密钥对的数字索引（如 `gss list` 所示）。如果未提供，GSS 将进入交互模式，提示你从列表中选择。
  * `-s <scope>`：**可选**。指定 Git 配置更新的范围。可以是 `global`（默认）或 `local`。
      * `global`：更新 `~/.gitconfig` 文件。
      * `local`：更新当前仓库中的 `.git/config` 文件。如果使用 `local` 范围，请确保你在一个 Git 仓库中。

切换时，GSS 会：

1.  更新 `~/.ssh/config` 以使用所选密钥的私钥。
2.  将 Git 配置中的 `core.sshCommand` 设置为指向 GSS 管理的 `~/.ssh/config`。
3.  根据导入密钥的设置更新 Git 配置中的 `user.name` 和 `user.email`。

**示例：**

```bash
gss s -i 0             # 切换到索引为 0 的密钥（全局 Git 范围）
gss s                  # 进入交互模式选择密钥
gss s -i 1 -s local    # 切换到索引为 1 的密钥并更新本地 Git 配置
```

#### `gss delete | del [-i <index>] [-f]`

从 GSS 配置中删除 SSH 密钥对条目。**此命令仅将条目从 GSS 的管理中移除；它不会删除你系统上的实际私钥和公钥文件。**

  * `-i <index>`：**可选**。要删除的密钥对条目的数字索引。如果未提供，GSS 将进入交互模式，提示你从列表中选择。
  * `-f`：**可选**。强制删除，不进行确认提示。

**示例：**

```bash
gss del -i 2           # 删除索引为 2 的密钥条目
gss del                # 进入交互模式选择要删除的密钥
gss del -i 0 -f        # 强制删除索引为 0 的密钥条目，不进行确认
```

-----

## ⚙️ 配置

GSS 将其配置存储在 `~/.gss/config.json` 文件中。你通常不需要直接编辑此文件，因为所有操作都通过 `gss` 命令行工具进行管理。

-----

## 🤝 贡献

欢迎贡献！如果你发现 bug 或有功能请求，请提交一个 Issue。

-----