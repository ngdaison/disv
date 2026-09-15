import discord
from discord.ext import commands
import datetime
from dateutil import parser
import re
import asyncio
from utils.data_handler import load_data, save_data, get_guild_data, get_channel_settings
from utils.antispam import should_process

LINK_REGEX = re.compile(r'https?:\/\/\S+')
TIKTOK_REGEX = re.compile(r'https:\/\/(?:m|www|vt)?\.tiktok\.com\/\S+')

class Moderation(commands.Cog):
    def __init__(self, bot):
        self.bot = bot
        # Format: {(guild_id, user_id): [{"msg": discord.Message, "t": float, "c": str, "sig": str, "channel_id": int, "id": int}, ...]}
        self.user_history = {}
        self.last_penalty = {} # {(guild_id, user_id): float}

    async def delete_recent_messages(self, guild_id: int, user_id: int, current_message: discord.Message = None):
        """Xóa tất cả tin nhắn gần đây của người dùng trên toàn bộ các kênh trong server."""
        key = (guild_id, user_id)
        history = self.user_history.get(key, [])
        
        messages_to_delete = {}
        if current_message:
            messages_to_delete[current_message.id] = current_message
            
        for item in history:
            msg = item.get("msg")
            if msg and msg.id not in messages_to_delete:
                messages_to_delete[msg.id] = msg
                
        # Làm trống lịch sử để không xoá hay trigger lại
        self.user_history[key] = []
        
        async def _safe_delete(m):
            try:
                await m.delete()
            except (discord.NotFound, discord.Forbidden, discord.HTTPException):
                pass
            except Exception as e:
                print(f"Error deleting message {m.id}: {e}")

        if messages_to_delete:
            tasks = [_safe_delete(m) for m in messages_to_delete.values()]
            await asyncio.gather(*tasks, return_exceptions=True)

    @commands.Cog.listener()
    async def on_message(self, message):
        if message.author.bot or not message.guild:
            return

        # Nếu người dùng đang bị timeout, xóa ngay tin nhắn
        if isinstance(message.author, discord.Member) and message.author.is_timed_out():
            try:
                await message.delete()
            except Exception:
                pass
            return

        if not should_process(f"mod_{message.id}"):
            return

        if await self.process_filters(message):
            return

    async def apply_penalty(self, message, reason):
        # Debounce các trigger liên tiếp trong 1.5s cho cùng 1 user (tránh spam cảnh báo/file race)
        now_ts = datetime.datetime.now().timestamp()
        penalty_key = (message.guild.id, message.author.id)
        last_penalty_ts = self.last_penalty.get(penalty_key, 0)
        if now_ts - last_penalty_ts < 1.5:
            try:
                await message.delete()
            except Exception:
                pass
            return
        self.last_penalty[penalty_key] = now_ts

        # 6-Step Progressive System
        data = load_data()
        g_data = get_guild_data(data, message.guild.id)
        uid = str(message.author.id)
        
        if uid not in g_data["users"]: 
            g_data["users"][uid] = {"xp": 0, "level": 1, "warnings": []}
        
        u = g_data["users"][uid]
        now = datetime.datetime.now()
        
        # Reset Logic: If last warning was > 72h ago, reset to status of 0 warnings
        if u.get("warnings"):
            try:
                last_w = u["warnings"][-1]
                last_time = parser.parse(last_w['time'])
                if (now - last_time).total_seconds() > 72 * 3600:
                    u["warnings"] = []
            except Exception as e:
                print(f"Error parsing warning time: {e}")
                u["warnings"] = [] # Safe reset on error

        # Add new warning
        u["warnings"].append({"reason": reason, "time": now.isoformat()})
        count = len(u["warnings"])
        save_data(data)
        
        # Xóa TẤT CẢ tin nhắn vừa gửi gần đây của người đó trên mọi kênh
        await self.delete_recent_messages(message.guild.id, message.author.id, current_message=message)
        
        dm_prefix = f"⚠️ Bạn đã vi phạm tại **{message.guild.name}**: {reason}."
        
        # Penalty Ladder
        # 1: Warning
        # 2: Warning
        # 3: Mute 1h
        # 4: Mute 24h
        # 5: Mute 48h
        # 6+: Mute 168h (7 days)
        
        try:
            if count == 1:
                await message.author.send(f"{dm_prefix} Cảnh báo lần 1 (Nhắc nhở).")
            elif count == 2:
                await message.author.send(f"{dm_prefix} Cảnh báo lần 2 (Nhắc nhở).")
            elif count == 3:
                duration = datetime.timedelta(hours=1)
                await message.author.timeout(duration, reason="Vi phạm lần 3 (Mute 1h)")
                await message.author.send(f"{dm_prefix} Vi phạm lần 3: Mute 1 giờ.")
            elif count == 4:
                duration = datetime.timedelta(hours=24)
                await message.author.timeout(duration, reason="Vi phạm lần 4 (Mute 24h)")
                await message.author.send(f"{dm_prefix} Vi phạm lần 4: Mute 24 giờ.")
            elif count == 5:
                duration = datetime.timedelta(hours=48)
                await message.author.timeout(duration, reason="Vi phạm lần 5 (Mute 48h)")
                await message.author.send(f"{dm_prefix} Vi phạm lần 5: Mute 48 giờ.")
            elif count >= 6:
                duration = datetime.timedelta(hours=168)
                await message.author.timeout(duration, reason=f"Vi phạm lần {count} (Mute 1 tuần)")
                await message.author.send(f"{dm_prefix} Vi phạm lần {count}: Mute 7 ngày. (Reset sau 72h không vi phạm)")
        except discord.Forbidden:
             print(f"Missing permissions to punish user {uid}")
        except Exception as e:
             print(f"Failed to punish user {uid}: {e}")

        # Send Notification to Admin (Bot Owner)
        try:
            if not self.bot.owner_id:
                app_info = await self.bot.application_info()
                self.bot.owner_id = app_info.owner.id
                owner = app_info.owner
            else:
                owner = self.bot.get_user(self.bot.owner_id) or await self.bot.fetch_user(self.bot.owner_id)
            
            if owner:
                embed = discord.Embed(title="🛡️ Violation Report", color=discord.Color.red(), timestamp=now)
                embed.add_field(name="Server", value=f"{message.guild.name} ({message.guild.id})", inline=False)
                embed.add_field(name="User", value=f"{message.author} ({message.author.id})", inline=False)
                embed.add_field(name="Reason", value=reason, inline=False)
                embed.add_field(name="Count", value=str(count), inline=True)
                embed.add_field(name="Action", value=f"Warning/Mute (Step {count})", inline=True)
                await owner.send(embed=embed)
        except Exception as e:
            print(f"Failed to send admin notification: {e}")

    async def process_filters(self, message):
        if not message.guild:
            return False

        data = load_data()
        g_data = get_guild_data(data, message.guild.id)
        c_settings = get_channel_settings(data, message.guild.id, message.channel.id)
        
        # Analyze Content
        content = message.content or ""
        attachments = message.attachments
        stickers = message.stickers
        
        has_link = bool(LINK_REGEX.search(content))
        has_media = any(a.content_type and (a.content_type.startswith('image') or a.content_type.startswith('video')) for a in attachments)
        has_file = any(a.content_type and not (a.content_type.startswith('image') or a.content_type.startswith('video')) for a in attachments)
        has_sticker = bool(stickers)

        # Lưu tin nhắn vào lịch sử gần đây của người dùng (theo dõi xuyên kênh trong server)
        uid = message.author.id
        gid = message.guild.id
        key = (gid, uid)
        now_ts = datetime.datetime.now().timestamp()
        
        if key not in self.user_history:
            self.user_history[key] = []
            
        # Dọn dẹp tin nhắn cũ hơn 60 giây
        self.user_history[key] = [x for x in self.user_history[key] if now_ts - x["t"] <= 60.0]
        
        norm_content = content.strip().lower()
        media_sig = None
        if not norm_content:
            if attachments:
                media_sig = f"att:{attachments[0].filename}:{attachments[0].size}"
            elif stickers:
                media_sig = f"stk:{stickers[0].id}"

        self.user_history[key].append({
            "msg": message,
            "t": now_ts,
            "c": norm_content,
            "sig": media_sig,
            "channel_id": message.channel.id,
            "id": message.id
        })
        self.user_history[key] = self.user_history[key][-20:]
        history = self.user_history[key]
        
        # 1. Anti-Link
        if c_settings.get('ac_link') and has_link:
            is_tiktok_allowed = bool(TIKTOK_REGEX.search(content)) and c_settings.get('tiktok_enabled')
            if not is_tiktok_allowed:
                await self.apply_penalty(message, "Gửi link (Cấm)")
                return True

        # 2. Anti-Media
        if c_settings.get('ac_media') and has_media:
            await self.apply_penalty(message, "Gửi Media (Cấm)")
            return True

        # 3. Anti-File
        if c_settings.get('ac_file') and has_file:
            await self.apply_penalty(message, "Gửi file (Cấm)")
            return True

        # 4. Anti-Chat (Block Text)
        if c_settings.get('ac_text'):
             is_pure_text = not (has_link or has_media or has_file or has_sticker)
             if is_pure_text:
                 await self.apply_penalty(message, "Chat Text (Cấm)")
                 return True

        # Anti-Spam (Anti Fast & Anti Dup) - Liên kết tất cả các kênh
        is_fast_enabled = g_data.get('as_fast', False) or c_settings.get('as_fast', False)
        is_dup_enabled = g_data.get('as_dup', False) or c_settings.get('as_dup', False)

        # 5. Anti Fast Check
        if is_fast_enabled:
            # (a) Nhanh chung: >= 2 tin nhắn trong 1.5 giây (cùng kênh hoặc khác kênh)
            recent_fast = [x for x in history if now_ts - x["t"] <= 1.5]
            if len(recent_fast) >= 2:
                recent_channels = {x["channel_id"] for x in recent_fast}
                reason = "Spam nhanh (nhiều kênh)" if len(recent_channels) >= 2 else "Spam nhanh"
                await self.apply_penalty(message, reason)
                return True

            # (b) Gửi quá nhanh giữa các kênh khác nhau: >= 2 kênh trong 2.5 giây
            recent_cross = [x for x in history if now_ts - x["t"] <= 2.5]
            cross_channels = {x["channel_id"] for x in recent_cross}
            if len(cross_channels) >= 2 and len(recent_cross) >= 2:
                await self.apply_penalty(message, "Spam nhanh (nhiều kênh)")
                return True

        # 6. Anti Dup Check
        if is_dup_enabled:
            is_tiktok_link = bool(TIKTOK_REGEX.search(content)) and c_settings.get('tiktok_enabled')

            # Kiểm tra lặp nội dung text (trong 60 giây)
            if norm_content:
                dups = [x for x in history if x.get("c") == norm_content and now_ts - x["t"] <= 60.0]
                if len(dups) >= 2:
                    dup_channels = {x["channel_id"] for x in dups}

                    # (a) Khác kênh: gửi cùng nội dung ở >= 2 kênh khác nhau trong 60s -> Luôn là Spam xuyên kênh
                    if len(dup_channels) >= 2:
                        await self.apply_penalty(message, "Spam lặp lại (nhiều kênh)")
                        return True

                    # (b) Cùng kênh:
                    # - Nếu là link TikTok trong kênh bật TikTok -> Cho phép, không xóa và không coi là spam!
                    # - Nếu là tin nhắn thường: Chỉ phạt nếu gửi quá nhanh (<= 3s) hoặc spam liên tục (>= 3 tin trong 30s)
                    elif not is_tiktok_link:
                        recent_same_dups = [x for x in dups if now_ts - x["t"] <= 3.0]
                        continuous_dups = [x for x in dups if now_ts - x["t"] <= 30.0]
                        if len(recent_same_dups) >= 2 or len(continuous_dups) >= 3:
                            await self.apply_penalty(message, "Spam lặp lại")
                            return True

            # Kiểm tra lặp media/sticker (trong 60 giây)
            elif media_sig:
                dups = [x for x in history if x.get("sig") == media_sig and now_ts - x["t"] <= 60.0]
                if len(dups) >= 2:
                    dup_channels = {x["channel_id"] for x in dups}
                    if len(dup_channels) >= 2:
                        await self.apply_penalty(message, "Spam lặp lại media (nhiều kênh)")
                        return True
                    else:
                        recent_same_media = [x for x in dups if now_ts - x["t"] <= 3.0]
                        if len(recent_same_media) >= 2 or len(dups) >= 3:
                            await self.apply_penalty(message, "Spam lặp lại media")
                            return True

        return False

async def setup(bot):
    await bot.add_cog(Moderation(bot))
