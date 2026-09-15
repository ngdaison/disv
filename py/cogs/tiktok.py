import discord
from discord.ext import commands
import re
import os
import aiohttp
import json
import asyncio
import time
from datetime import datetime, timezone
from utils.data_handler import load_data, save_data, get_guild_data, get_channel_settings, VIDEO_FOLDER
from utils.antispam import should_process

import subprocess

TIKTOK_REGEX = re.compile(r'https:\/\/(?:m|www|vt)?\.tiktok\.com\/\S+')

HEADERS = {
    "User-Agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/122.0.0.0 Safari/537.36",
    "Referer": "https://www.tiktok.com/",
    "Accept": "*/*"
}

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
        return 30.0

def compress_video_ultrafast(input_path, output_path, max_size_bytes):
    try:
        duration = get_video_duration(input_path)
        if duration <= 0:
            duration = 30.0

        target_size_bytes = int(max_size_bytes * 0.92)
        audio_bits = 128 * 1000 * duration
        remaining_bits = target_size_bytes * 8 - audio_bits
        video_bitrate_kbps = max(200, min(6000, int(remaining_bits / duration / 1000)))

        temp_output = f"{output_path}.tmp.{int(time.time() * 1000)}.mp4"

        cmd = [
            'ffmpeg', '-y',
            '-i', input_path,
            '-c:v', 'libx264',
            '-preset', 'veryfast',
            '-b:v', f'{video_bitrate_kbps}k',
            '-c:a', 'copy',
            '-movflags', '+faststart',
            temp_output
        ]

        res = subprocess.run(cmd, capture_output=True, timeout=60)
        if res.returncode != 0:
            if os.path.exists(temp_output):
                try: os.remove(temp_output)
                except: pass
            return False

        if os.path.exists(temp_output) and os.path.getsize(temp_output) > 0:
            if os.path.exists(output_path):
                try: os.remove(output_path)
                except: pass
            os.rename(temp_output, output_path)
            return True
        return False
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

        if message.created_at and (datetime.now(timezone.utc) - message.created_at).total_seconds() > 120:
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
            # Phản hồi ngay lập tức với typing (delay ~ 0)
            async with message.channel.typing():
                async with self.bot.http_session.get("https://www.tikwm.com/api/", params={"url": url, "hd": "1"}, headers=HEADERS, timeout=15) as resp:
                    if resp.status != 200:
                        return
                    api_data = await resp.json()
                
                v_data = api_data.get("data")
                if not v_data:
                    return

                vid = v_data.get("id", "video")
                g_data = get_guild_data(data, message.guild.id)
                if vid in g_data.get("bad_videos", []):
                    return

                # Xử lý hình ảnh (Slides / Photo mode)
                if v_data.get("images"):
                    folder_path = os.path.join(VIDEO_FOLDER, vid)
                    os.makedirs(folder_path, exist_ok=True)
                    
                    # Tải song song audio và toàn bộ ảnh
                    async def fetch_file(file_url, dest_path):
                        if not os.path.exists(dest_path) or os.path.getsize(dest_path) == 0:
                            try:
                                temp = f"{dest_path}.tmp.{int(time.time()*1000)}"
                                async with self.bot.http_session.get(file_url, headers=HEADERS) as r:
                                    if r.status == 200:
                                        content = await r.read()
                                        if len(content) > 0:
                                            with open(temp, "wb") as f:
                                                f.write(content)
                                            if os.path.exists(dest_path): os.remove(dest_path)
                                            os.rename(temp, dest_path)
                            except Exception as ex:
                                print(f"Error fetching {file_url}: {ex}")

                    tasks = []
                    aud = v_data.get("play") or v_data.get("music")
                    audio_path = os.path.join(folder_path, "audio.mp3") if aud else None
                    if aud:
                        aud_url = aud if aud.startswith("http") else f"https://www.tikwm.com{aud}"
                        tasks.append(fetch_file(aud_url, audio_path))

                    img_targets = []
                    for i, img_url in enumerate(v_data["images"]):
                        if not img_url.startswith("http"):
                            img_url = f"https://www.tikwm.com{img_url}"
                        img_dest = os.path.join(folder_path, f"{i}.jpg")
                        img_targets.append((f"image_{i+1}.jpg", img_dest))
                        tasks.append(fetch_file(img_url, img_dest))

                    await asyncio.gather(*tasks)

                    audio_file = audio_path if (audio_path and os.path.exists(audio_path) and os.path.getsize(audio_path) > 0) else None
                    valid_imgs = [(name, path) for name, path in img_targets if os.path.exists(path) and os.path.getsize(path) > 0]

                    if not valid_imgs and not audio_file:
                        return

                    batches = []
                    cur_batch = []
                    cur_size = 0

                    if audio_file:
                        cur_batch.append(discord.File(audio_file, filename="audio.mp3"))
                        cur_size += os.path.getsize(audio_file)

                    for name, path in valid_imgs:
                        f_size = os.path.getsize(path)
                        if cur_batch and (len(cur_batch) >= 10 or cur_size + f_size > 7500000):
                            batches.append(cur_batch)
                            cur_batch = []
                            cur_size = 0
                        cur_batch.append(discord.File(path, filename=name))
                        cur_size += f_size

                    if cur_batch:
                        batches.append(cur_batch)

                    for batch in batches:
                        await message.reply(files=batch, mention_author=True)
                        await asyncio.sleep(0.2)

                    try: await message.edit(suppress=True)
                    except: pass
                    return

                # Xử lý video thường: Luôn luôn ưu tiên chất lượng cao nhất (hdplay)
                link = v_data.get("hdplay") or v_data.get("play") or v_data.get("wmplay")
                if not link:
                    return
                
                is_hd = bool(v_data.get("hdplay"))
                file_name = f"{vid}_hd.mp4" if is_hd else f"{vid}.mp4"
                link = link if link.startswith("http") else f"https://www.tikwm.com{link}"
                file_path = os.path.join(VIDEO_FOLDER, file_name)
                
                if not os.path.exists(file_path) or os.path.getsize(file_path) == 0:
                    temp_file = f"{file_path}.tmp.{int(time.time() * 1000)}"
                    async with self.bot.http_session.get(link, headers=HEADERS) as r:
                        if r.status == 200:
                            content = await r.read()
                            if len(content) > 0:
                                with open(temp_file, "wb") as f:
                                    f.write(content)
                                if os.path.exists(file_path):
                                    try: os.remove(file_path)
                                    except: pass
                                os.rename(temp_file, file_path)

                if not os.path.exists(file_path) or os.path.getsize(file_path) == 0:
                    return

                max_size = message.guild.filesize_limit if message.guild else (10 * 1024 * 1024)
                file_size = os.path.getsize(file_path)

                send_file_path = file_path

                # Nếu video dưới ngưỡng dung lượng: GỬI NGAY (0s delay, không chạy ffmpeg)
                if file_size > max_size:
                    max_size_mb = int(max_size / (1024 * 1024))
                    compressed_name = f"{vid}_hd_fast_{max_size_mb}mb.mp4" if is_hd else f"{vid}_fast_{max_size_mb}mb.mp4"
                    compressed_path = os.path.join(VIDEO_FOLDER, compressed_name)
                    
                    if not os.path.exists(compressed_path) or os.path.getsize(compressed_path) == 0 or os.path.getsize(compressed_path) > max_size:
                        await asyncio.to_thread(compress_video_ultrafast, file_path, compressed_path, max_size)
                    
                    if os.path.exists(compressed_path) and os.path.getsize(compressed_path) > 0 and os.path.getsize(compressed_path) <= max_size:
                        send_file_path = compressed_path
                    else:
                        await message.reply(content=link, mention_author=True)
                        try: await message.edit(suppress=True)
                        except: pass
                        return

                await message.reply(
                    file=discord.File(send_file_path, filename=f"{vid}.mp4"),
                    mention_author=True 
                )
                try: await message.edit(suppress=True)
                except: pass

        except Exception as e:
            print(f"TikTok Error: {e}")

async def setup(bot):
    await bot.add_cog(TikTok(bot))
