import discord
from discord.ext import commands
import re
import os
import aiohttp
import json
import asyncio
from utils.data_handler import load_data, save_data, get_guild_data, get_channel_settings, VIDEO_FOLDER
from utils.antispam import should_process

import subprocess

TIKTOK_REGEX = re.compile(r'https:\/\/(?:m|www|vt)?\.tiktok\.com\/\S+')


def get_video_duration(input_path):
    try:
        cmd = [
            'ffprobe', '-v', 'error',
            '-show_entries', 'format=duration',
            '-of', 'default=noprint_wrappers=1:nokey=1',
            input_path
        ]
        res = subprocess.run(cmd, capture_output=True, text=True, timeout=10)
        return float(res.stdout.strip())
    except Exception as e:
        print(f"ffprobe error: {e}")
        return 0.0

def compress_video(input_path, output_path, max_size_bytes):
    try:
        duration = get_video_duration(input_path)
        if duration <= 0:
            duration = 30.0

        max_size_mb = max_size_bytes / (1024 * 1024)
        # Mục tiêu dung lượng an toàn (~90% max_size_bytes để chắc chắn dưới ngưỡng của Discord)
        target_size_bytes = int(max_size_bytes * 0.90)
        audio_bitrate_kbps = 96
        audio_bits = audio_bitrate_kbps * 1000 * duration

        remaining_bits = target_size_bytes * 8 - audio_bits
        if remaining_bits < 100000 * duration:
            video_bitrate_kbps = 150
        else:
            video_bitrate_kbps = int(remaining_bits / duration / 1000)

        # Điều chỉnh độ phân giải và trần bitrate phù hợp với hạn mức server (10MB, 50MB hoặc 100MB)
        if max_size_mb >= 80:
            max_cap_bitrate = 18000
            scale_filter = r"scale=-2:'min(1080,ih)'"
        elif max_size_mb >= 40:
            max_cap_bitrate = 12000
            scale_filter = r"scale=-2:'min(1080,ih)'"
        else:
            max_cap_bitrate = 3000
            scale_filter = r"scale=-2:'min(720,ih)'"

        video_bitrate_kbps = max(150, min(video_bitrate_kbps, max_cap_bitrate))
        maxrate_kbps = int(video_bitrate_kbps * 1.15)
        bufsize_kbps = video_bitrate_kbps * 2

        cmd = [
            'ffmpeg', '-y',
            '-i', input_path,
            '-c:v', 'libx264',
            '-b:v', f'{video_bitrate_kbps}k',
            '-maxrate', f'{maxrate_kbps}k',
            '-bufsize', f'{bufsize_kbps}k',
            '-vf', scale_filter,
            '-preset', 'faster',
            '-c:a', 'aac',
            '-b:a', f'{audio_bitrate_kbps}k',
            output_path
        ]

        res = subprocess.run(cmd, capture_output=True, timeout=180)
        if res.returncode != 0:
            print(f"FFmpeg compress error: {res.stderr.decode('utf-8', errors='ignore')}")
            return False

        return os.path.exists(output_path) and os.path.getsize(output_path) > 0
    except Exception as e:
        print(f"Compress Error: {e}")
        return False

class TikTok(commands.Cog):
    def __init__(self, bot):
        self.bot = bot
        if not os.path.exists(VIDEO_FOLDER):
            os.makedirs(VIDEO_FOLDER)

    @commands.Cog.listener()
    async def on_message(self, message):
        if message.author.bot:
            return
        
        match = TIKTOK_REGEX.search(message.content)
        if not match:
            return

        data = load_data()
        c_settings = get_channel_settings(data, message.guild.id, message.channel.id)
        if not c_settings['tiktok_enabled']:
            return

        if not should_process(f"tiktok_{message.id}"):
            return

        url = match.group(0)
        await self.process_tiktok_video(message, url, data)

    async def process_tiktok_video(self, message, url, data):
        try:
            async with self.bot.http_session.get("https://www.tikwm.com/api/", params={"url": url, "hd": 1}, timeout=20) as resp:
                api_data = await resp.json()
            
            v_data = api_data.get("data")
            if not v_data:
                return

            vid = v_data.get("id", "video")
            g_data = get_guild_data(data, message.guild.id)
            if vid in g_data.get("bad_videos", []):
                return

            # Xử lý hình ảnh (Slides)
            if v_data.get("images"):
                folder_path = os.path.join(VIDEO_FOLDER, vid)
                os.makedirs(folder_path, exist_ok=True)
                
                files_to_send = []
                
                # Audio
                aud = v_data.get("play") or v_data.get("music")
                if aud:
                    aud_url = aud if aud.startswith("http") else f"https://www.tikwm.com{aud}"
                    aud_path = os.path.join(folder_path, "audio.mp3")
                    if not os.path.exists(aud_path):
                        async with self.bot.http_session.get(aud_url) as r:
                            with open(aud_path, "wb") as f:
                                f.write(await r.read())
                    files_to_send.append(discord.File(aud_path, filename="audio.mp3"))

                # Images
                for i, img_url in enumerate(v_data["images"]):
                    img_path = os.path.join(folder_path, f"{i}.jpg")
                    if not os.path.exists(img_path):
                        async with self.bot.http_session.get(img_url) as r:
                            with open(img_path, "wb") as f:
                                f.write(await r.read())
                    files_to_send.append(discord.File(img_path, filename=f"image_{i}.jpg"))
                
                for i in range(0, len(files_to_send), 10):
                    chunk = files_to_send[i:i+10]
                    await message.reply(files=chunk, mention_author=True)
                
                try: await message.edit(suppress=True)
                except: pass
                
                return

            # Xử lý video thường
            link = v_data.get("play") or v_data.get("hdplay") or v_data.get("wmplay")
            if not link:
                return
            
            link = link if link.startswith("http") else f"https://www.tikwm.com{link}"
            file_path = os.path.join(VIDEO_FOLDER, f"{vid}.mp4")
            
            if not os.path.exists(file_path):
                async with self.bot.http_session.get(link) as r:
                    content = await r.read()
                    with open(file_path, "wb") as f:
                        f.write(content)
                        
            # Lấy giới hạn dung lượng tải lên của server (tự động nhận Boost Server: 10MB, 50MB hoặc 100MB)
            max_size = message.guild.filesize_limit if message.guild else (10 * 1024 * 1024)  # Trả về số byte: 10MB, 50MB hoặc 100MB tùy server

            # Kiểm tra kích thước video gốc
            file_size = os.path.getsize(file_path)
            
            # Nếu video vượt quá giới hạn server thì mới nén xuống dưới giới hạn tương ứng
            if file_size > max_size:
                max_size_mb = int(max_size / (1024 * 1024))
                compressed_path = os.path.join(VIDEO_FOLDER, f"{vid}_compressed_{max_size_mb}mb.mp4")
                
                # Nén nếu chưa có file nén phù hợp hoặc file nén cũ vẫn vượt ngưỡng
                if not os.path.exists(compressed_path) or os.path.getsize(compressed_path) > max_size:
                    await asyncio.to_thread(compress_video, file_path, compressed_path, max_size)
                
                if os.path.exists(compressed_path) and os.path.getsize(compressed_path) > 0:
                    file_path = compressed_path

            # Kiểm tra dung lượng thực tế sau cùng
            final_size = os.path.getsize(file_path)
            if final_size > max_size:
                # Nếu sau khi nén vẫn vượt giới hạn server, gửi link direct để Discord nhúng video player xem trực tiếp
                await message.reply(
                    content=link,
                    mention_author=True
                )
                try: await message.edit(suppress=True)
                except: pass
                return

            await message.reply(
                file=discord.File(file_path),
                mention_author=True 
            )
            try: await message.edit(suppress=True)
            except: pass

        except Exception as e:
            print(f"TikTok Error: {e}")

async def setup(bot):
    await bot.add_cog(TikTok(bot))
