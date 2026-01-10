# P2P 聊天程序

一个基于Go语言实现的命令行P2P（点对点）聊天程序，支持跨局域网通信、消息加密和房间系统。

## 功能特点

- **P2P架构**：节点间直接通信，无需中心服务器
- **NAT穿透**：使用STUN协议实现跨局域网通信
- **消息加密**：AES-128对称加密确保消息安全
- **房间系统**：支持创建和加入聊天房间
- **配置文件**：支持自定义配置参数
- **跨平台**：支持Windows、Linux、macOS

## 安装

```bash
# 克隆项目
git clone <repository-url>
cd p2pchat

# 构建程序
go build -o chat .
```

## 配置文件

程序使用 `config` 文件进行配置（无后缀名），包含以下参数：

```ini
TCPPORT=8080                    # TCP监听端口
UDPPORT=8081                    # UDP广播端口
BROADCAST_TIMEOUT=5s            # 广播超时时间
DEFAULT_NICKNAME=               # 默认昵称（留空则自动生成）
DEFAULT_ADJECTIVES=Cool,Smart,Fast,Lucky,Brave,Clever,Quick,Sharp,Bright,Wise
                                # 生成昵称的形容词列表
DEFAULT_NOUNS=Tiger,Eagle,Wolf,Fox,Bear,Hawk,Lion,Shark,Horse,Owl
                                # 生成昵称的名词列表
MAX_NODES=100                   # 最大节点数
FILE_CHUNK_SIZE=1024            # 文件块大小（字节）
```

## 使用方法

### 1. 创建房间

```bash
./p2pchat
> /create myroom
```

程序将创建名为 `myroom` 的房间，并显示房间密钥：

```
Room created successfully! Room ID: myroom
Room key: [base64 encoded key]
Your nickname: [generated nickname]
```

### 2. 加入房间

```bash
./p2pchat
> /join myroom [房间密钥]
```

其他节点可以使用房间ID和密钥加入房间。

### 3. 发送消息

直接输入消息（不带/前缀）即可发送：

```bash
> Hello, everyone!
```

### 4. 查看房间节点

```bash
> /list
```

### 5. 其他功能

- `> /save` - 保存聊天记录到文件
- `> /file [文件路径]` - 发送文件
- `> /help` - 显示帮助信息
- `> /exit` - 退出程序

## 命令说明

| 命令 | 说明 |
|------|------|
| `/create [房间ID]` | 创建新房间 |
| `/join [房间ID] [密钥]` | 加入指定房间 |
| `消息内容（无/前缀）` | 发送聊天消息 |
| `/list` | 列出房间内节点 |
| `/save` | 保存聊天记录 |
| `/file [文件路径]` | 发送文件 |
| `/help` | 显示帮助信息 |
| `/exit` | 退出程序 |

## 工作原理

### P2P通信

1. **节点发现**：使用UDP广播在局域网内发现其他节点
2. **NAT穿透**：通过STUN服务器获取公网IP和端口，实现跨局域网通信
3. **消息传输**：使用TCP协议保证消息可靠传输

### 消息加密

- 使用AES-128算法，CBC模式+PKCS7填充
- 房间创建者生成16字节密钥，加入者需输入相同密钥才能解密消息
- 所有消息在传输前进行加密

### 房间系统

- 房间创建者维护房间内所有节点列表
- 新节点通过UDP广播同步到房间内其他节点
- 消息仅向房间内节点广播

## 安全性

- 所有消息使用AES-128加密传输
- 房间密钥确保只有授权用户可加入
- 支持昵称自定义，保护用户隐私

## 注意事项

- 确保防火墙允许TCP/UDP端口通信
- STUN服务器用于NAT穿透，需要网络连接
- 配置文件修改后需重启程序生效

## 技术栈

- 语言：Go 1.25+
- 标准库：net, crypto, encoding, bufio, os
- 协议：UDP, TCP, STUN
- 加密：AES-128-CBC