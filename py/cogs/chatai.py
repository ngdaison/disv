import discord
from discord.ext import commands
import json
import os
import aiohttp

from utils.data_handler import load_data, save_data, get_channel_settings
from utils.antispam import should_process


class ChatAI(commands.Cog):
    def __init__(self, bot):
        self.bot = bot

    def _split_message(self, text, limit=2000):
        # Split text into chunks no longer than `limit`, prefer splitting on newlines or spaces
        if not text:
            return []
        chunks = []
        while text:
            if len(text) <= limit:
                chunks.append(text)
                break
            # try split at last double newline within limit
            idx = text.rfind('\n\n', 0, limit)
            if idx == -1:
                idx = text.rfind('\n', 0, limit)
            if idx == -1:
                idx = text.rfind(' ', 0, limit)
            if idx == -1:
                # forced split
                idx = limit
            chunk = text[:idx].rstrip()
            if not chunk:
                chunk = text[:limit]
                text = text[limit:]
            else:
                text = text[idx:].lstrip()
            chunks.append(chunk)
        return chunks

    @commands.Cog.listener()
    async def on_message(self, message: discord.Message):
        if message.author.bot:
            return

        # Prevent processing DMs
        if not message.guild:
            return

        if not should_process(f"ai_{message.id}"):
            return

        all_data = load_data()
        c_settings = get_channel_settings(all_data, message.guild.id, message.channel.id)
        print(f"[ChatAI] Message from {message.author} in {message.guild}/{message.channel}: '{message.content[:80]}'")
        print(f"[ChatAI] Channel ai_enabled={c_settings.get('ai_enabled')}")
        if not c_settings.get("ai_enabled"):
            print("[ChatAI] AI not enabled for this channel - skipping")
            return

        # Load config
        try:
            with open('config.json', 'r', encoding='utf-8') as f:
                cfg = json.load(f)
            API_KEY = cfg.get('ai_api_key', '')
            LOCAL_AI_URL = cfg.get('local_ai_url', 'http://localhost:6660/api')
        except Exception:
            API_KEY = ''
            LOCAL_AI_URL = 'http://localhost:6660/api'

        if not API_KEY and not LOCAL_AI_URL:
            print("[ChatAI] No AI API or Local AI configured")
            return

        if not hasattr(self, '_processed_msg_ids'):
            self._processed_msg_ids = set()
        if message.id in self._processed_msg_ids:
            return
        self._processed_msg_ids.add(message.id)
        if len(self._processed_msg_ids) > 1000:
            self._processed_msg_ids.clear()

        # Load system prompt if available
        system_prompt = ""
        if os.path.exists('train.txt'):
            try:
                with open('train.txt', 'r', encoding='utf-8') as f:
                    system_prompt = f.read()
            except Exception:
                system_prompt = ""

        # Lấy tối đa 10 tin nhắn gần nhất trước tin nhắn này trong kênh
        history_lines = []
        try:
            async for old_msg in message.channel.history(limit=10, before=message):
                if old_msg and old_msg.content:
                    c = old_msg.content.strip()
                    if c.startswith('/') or c.startswith('!') or c.startswith('.'):
                        continue
                    sender = "AI KiyoVN" if (self.bot.user and old_msg.author.id == self.bot.user.id) else old_msg.author.display_name
                    history_lines.insert(0, f"- {sender}: {c}")
        except Exception as e:
            print(f"[ChatAI] Lỗi lấy lịch sử tin nhắn: {e}")

        prompt_parts = []
        if system_prompt:
            prompt_parts.append(system_prompt)
            prompt_parts.append("\n\n")

        if history_lines:
            prompt_parts.append("Ngữ cảnh lịch sử trò chuyện gần đây trong kênh (tối đa 10 tin nhắn trước):\n")
            for h in history_lines:
                prompt_parts.append(h + "\n")
            prompt_parts.append("\n")

        prompt_parts.append(f"Tin nhắn mới nhất từ {message.author.display_name}:\n{message.content}\n\n")
        prompt_parts.append("Dựa trên toàn bộ thông tin về KiyoVN và ngữ cảnh lịch sử trò chuyện ở trên, hãy trả lời tin nhắn mới nhất thật ngắn gọn, chính xác, tự nhiên và đúng trọng tâm:")
        full_prompt = "".join(prompt_parts)
        reply = ''

        # 1. Try Local AI API first
        if LOCAL_AI_URL:
            local_endpoint = LOCAL_AI_URL.rstrip('/') + '/generate'
            try:
                print(f"[ChatAI] Sending request to Local AI API ({local_endpoint})")
                async with self.bot.http_session.post(local_endpoint, json={"prompt": full_prompt}, headers={"Content-Type": "application/json"}, timeout=aiohttp.ClientTimeout(total=45)) as resp:
                    if resp.status == 200:
                        resp_data = await resp.json()
                        reply = resp_data.get('text', '').strip()
                        print(f"[ChatAI] Local AI reply received, length={len(reply)}")
                    else:
                        print(f"[ChatAI] Local AI error status={resp.status}")
            except Exception as e:
                print(f"[ChatAI] Local AI request exception: {e}")

        # 2. Fallback to Gemini if no reply and API_KEY configured
        if not reply and API_KEY:
            URL = f"https://generativelanguage.googleapis.com/v1beta/models/gemini-2.5-flash-lite:generateContent?key={API_KEY}"
            payload = {
                "contents": [
                    {
                        "role": "user",
                        "parts": [
                            {
                                "text": full_prompt
                            }
                        ]
                    }
                ]
            }

            try:
                print(f"[ChatAI] Sending request to Gemini API (URL={URL})")
                async with self.bot.http_session.post(URL, json=payload, headers={"Content-Type": "application/json"}, timeout=aiohttp.ClientTimeout(total=30)) as resp:
                    resp_text = await resp.text()
                    print(f"[ChatAI] Gemini API responded status={resp.status}")
                    if resp.status == 200:
                        try:
                            resp_data = await resp.json()
                            reply = resp_data.get('candidates', [])[0].get('content', {}).get('parts', [])[0].get('text', '').strip()
                        except Exception:
                            pass
                    else:
                        print(f"[ChatAI] Gemini API error status={resp.status} text={resp_text}")
            except Exception as e:
                print(f"[ChatAI] Gemini request exception: {e}")

        if not reply:
            print("[ChatAI] Empty reply from AI")
            return

        # Decide which bot instance should send the reply.
        try:
            is_chatai_instance = getattr(self.bot, "is_chatai", False)
            print(f"[ChatAI] is_chatai_instance={is_chatai_instance}")
            if not is_chatai_instance:
                chatai_bot = getattr(self.bot, "chatai_bot", None)
                has_chatai_member = False
                if chatai_bot and getattr(chatai_bot, "user", None):
                    try:
                        has_chatai_member = bool(message.guild.get_member(chatai_bot.user.id))
                    except Exception:
                        has_chatai_member = False
                print(f"[ChatAI] chatai_bot present={bool(chatai_bot)}, chatai_member_in_guild={has_chatai_member}")
                if chatai_bot and has_chatai_member:
                    print("[ChatAI] chatai bot is present in guild - skipping reply from this bot")
                    return

            # Split long replies into Discord-safe chunks and send sequentially
            chunks = self._split_message(reply, limit=2000)
            if not chunks:
                print("[ChatAI] No chunks to send")
                return
            print(f"[ChatAI] Sending {len(chunks)} chunk(s) to channel {message.channel}")
            for i, chunk in enumerate(chunks):
                try:
                    if i == 0:
                        await message.reply(chunk, mention_author=False)
                    else:
                        await message.channel.send(chunk)
                except Exception as e:
                    print(f"[ChatAI] Failed sending chunk {i}: {e}")
                    return
            print("[ChatAI] Reply sent")
        except Exception as e:
            print(f"[ChatAI] Failed sending AI reply: {e}")
            return


async def setup(bot):
    await bot.add_cog(ChatAI(bot))
