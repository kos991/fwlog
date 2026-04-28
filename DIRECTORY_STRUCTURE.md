# 数据目录结构说明

## 目录结构

```
D:\sql\
├── sangforfw_log.sh              # 主脚本
├── fwlog                         # 快捷键包装脚本
├── data/                         # 数据根目录（新增）
│   ├── index/                    # 索引文件目录
│   │   ├── sangfor_fw_log_index.db      # 索引数据库
│   │   ├── sangfor_fw_log_index.db.gz   # 压缩索引
│   │   └── sangfor_fw_log_index.meta    # 索引元数据
│   ├── export/                   # 导出文件根目录
│   │   ├── by_ip/                # 按IP分类的查询结果
│   │   ├── by_port/              # 按端口分类的查询结果
│   │   ├── by_date/              # 按日期分类的查询结果
│   │   └── by_query/             # 组合查询结果
│   ├── temp/                     # 临时文件目录
│   └── backup/                   # 备份目录
├── README.md
├── FEATURE_CHECKLIST.md
└── 其他文档...
```

## 目录说明

### 1. data/index/ - 索引文件目录
存储所有索引相关文件：
- `sangfor_fw_log_index.db` - 主索引数据库
- `sangfor_fw_log_index.db.gz` - 压缩后的索引（节省空间）
- `sangfor_fw_log_index.meta` - 索引元数据（文件大小、修改时间等）

### 2. data/export/ - 导出文件根目录
所有查询结果按类型自动分类存储：

#### 2.1 by_ip/ - IP查询结果
存储以下类型的查询结果：
- 单IP查询
- 双IP查询（源IP+目的IP）

文件命名格式：`{IP}_{时间戳}.log`
示例：`192.168.1.100_20260427_143025.log`

#### 2.2 by_port/ - 端口查询结果
存储以下类型的查询结果：
- 端口查询（源端口/目的端口/任意端口）
- 协议查询（TCP/UDP/ICMP）

文件命名格式：`port_{端口号}_{时间戳}.log` 或 `proto_{协议号}_{时间戳}.log`
示例：
- `port_443_20260427_143025.log`
- `proto_6_20260427_143025.log`

#### 2.3 by_date/ - 日期查询结果
存储以下类型的查询结果：
- 时间范围查询
- 日期区间查询

文件命名格式：`time_{开始日期}_{结束日期}_{时间戳}.log`
示例：`time_20260424_20260427_143025.log`

#### 2.4 by_query/ - 组合查询结果
存储以下类型的查询结果：
- 组合查询（IP+端口+时间）
- 关键字查询
- 其他复杂查询

文件命名格式：`{查询关键字}_{时间戳}.log`
示例：`192.168.1.100_20260427_143025.log`

### 3. data/temp/ - 临时文件目录
存储查询过程中的临时文件，脚本会自动清理。

### 4. data/backup/ - 备份目录
预留用于索引备份（未来功能）。

## 自动分类规则

脚本会根据查询类型自动将结果存储到对应目录：

| 查询类型 | 目标目录 | 判断依据 |
|---------|---------|---------|
| 单IP查询 | by_ip/ | 查询描述包含"类型=单IP查询" |
| 双IP查询 | by_ip/ | 查询描述包含"类型=双IP查询" |
| 端口查询 | by_port/ | 查询描述包含"类型=端口查询" |
| 协议查询 | by_port/ | 查询描述包含"类型=端口查询" |
| 日期查询 | by_date/ | 查询描述包含"类型=日期查询" |
| 组合查询 | by_query/ | 查询描述包含"类型=组合查询" |
| 关键字查询 | by_query/ | 其他类型 |

## 优势

### 1. 结构清晰
- 所有数据文件集中在 `data/` 目录
- 按功能分类，易于管理和查找

### 2. 便于维护
- 索引文件独立存储
- 导出结果按类型分类
- 临时文件自动清理

### 3. 便于备份
- 只需备份 `data/` 目录即可
- 可以单独备份索引或导出结果

### 4. 便于清理
```bash
# 清理所有导出结果
rm -rf data/export/*

# 清理特定类型的导出结果
rm -rf data/export/by_ip/*

# 清理临时文件
rm -rf data/temp/*

# 重建索引（删除旧索引）
rm -rf data/index/*
```

## 迁移说明

### 从旧版本迁移

如果你之前使用的是旧版本（索引文件在脚本根目录），脚本会自动创建新的目录结构。旧文件不会自动迁移，你可以手动迁移：

```bash
# 迁移索引文件
mv sangfor_fw_log_index.db data/index/
mv sangfor_fw_log_index.db.gz data/index/
mv sangfor_fw_log_index.meta data/index/

# 迁移导出文件（需要手动分类）
# 建议重新查询，让脚本自动分类
```

### 首次使用

首次运行脚本时，会自动创建所有必要的目录结构，无需手动创建。

## 配置

目录结构在脚本中定义，如需修改，编辑 `sangforfw_log.sh` 的以下部分：

```bash
# 数据目录结构
DATA_DIR="$SCRIPT_DIR/data"
INDEX_DIR="$DATA_DIR/index"
EXPORT_DIR="$DATA_DIR/export"
EXPORT_BY_IP="$EXPORT_DIR/by_ip"
EXPORT_BY_PORT="$EXPORT_DIR/by_port"
EXPORT_BY_DATE="$EXPORT_DIR/by_date"
EXPORT_BY_QUERY="$EXPORT_DIR/by_query"
TEMP_DIR="$DATA_DIR/temp"
BACKUP_DIR="$DATA_DIR/backup"
```

## 注意事项

1. **权限要求**：确保脚本有权限在脚本目录下创建 `data/` 目录
2. **磁盘空间**：导出结果可能占用较多空间，定期清理旧文件
3. **备份建议**：定期备份 `data/index/` 目录，避免索引丢失
4. **临时文件**：`data/temp/` 目录中的文件会自动清理，不要存储重要数据

## 常见问题

### Q: 为什么要改变目录结构？
A: 新的目录结构更清晰、更易于管理，特别是当导出文件很多时，按类型分类可以快速找到需要的文件。

### Q: 旧的导出文件会自动迁移吗？
A: 不会。旧文件保持原位，新查询会使用新的目录结构。你可以手动迁移或重新查询。

### Q: 可以自定义目录位置吗？
A: 可以。编辑脚本中的目录定义部分即可。

### Q: 如何查看某个IP的所有历史查询？
A: 查看 `data/export/by_ip/` 目录，所有包含该IP的查询结果都在这里。

### Q: 临时文件会自动清理吗？
A: 是的。脚本执行完毕后会自动清理 `data/temp/` 中的临时文件。
