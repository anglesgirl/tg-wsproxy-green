#!/bin/bash
# Douyin_TikTok_Download_API 一键部署脚本
# 用法: bash deploy.sh [端口号]  默认端口 80

set -e

PORT=${1:-80}
WORK_DIR="$HOME/douyin_api"

echo "==> 创建工作目录: $WORK_DIR"
mkdir -p "$WORK_DIR"
cd "$WORK_DIR"

echo "==> 下载默认配置文件模板..."
curl -fsSL -o douyin_web_config.yaml \
  https://raw.githubusercontent.com/Evil0ctal/Douyin_TikTok_Download_API/main/crawlers/douyin/web/config.yaml
curl -fsSL -o tiktok_web_config.yaml \
  https://raw.githubusercontent.com/Evil0ctal/Douyin_TikTok_Download_API/main/crawlers/tiktok/web/config.yaml
curl -fsSL -o tiktok_app_config.yaml \
  https://raw.githubusercontent.com/Evil0ctal/Douyin_TikTok_Download_API/main/crawlers/tiktok/app/config.yaml

echo "==> 生成 docker-compose.yml (端口 $PORT)..."
cat > docker-compose.yml <<EOF
version: "3.9"
services:
  douyin_tiktok_download_api:
    image: evil0ctal/douyin_tiktok_download_api:latest
    container_name: douyin_tiktok_download_api
    restart: always
    ports:
      - "$PORT:80"
    volumes:
      - ./douyin_web_config.yaml:/app/crawlers/douyin/web/config.yaml
      - ./tiktok_web_config.yaml:/app/crawlers/tiktok/web/config.yaml
      - ./tiktok_app_config.yaml:/app/crawlers/tiktok/app/config.yaml
    environment:
      TZ: Asia/Shanghai
EOF

echo "==> 拉取镜像并启动..."
docker compose pull
docker compose up -d

echo ""
echo "==> 部署完成!"
echo "    API 文档:  http://<你的服务器IP>:$PORT/docs"
echo "    Web 界面:  http://<你的服务器IP>:$PORT/"
echo ""
echo "    下一步(必做): 编辑 $WORK_DIR/douyin_web_config.yaml"
echo "    替换 Cookie 字段后重启容器:"
echo "    docker compose restart"
echo ""
echo "    常用命令:"
echo "    查看日志:   docker compose logs -f"
echo "    停止:       docker compose down"
echo "    重启:       docker compose restart"
