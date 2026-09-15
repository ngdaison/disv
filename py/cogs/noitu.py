import discord
from discord.ext import commands
from discord import app_commands, ui
import sqlite3
import os
import random
import json
import asyncio
import datetime

def get_db_connection():
    candidates = ["dictionary.db", "../dictionary.db", "../../dictionary.db"]
    for p in candidates:
        if os.path.exists(p):
            return sqlite3.connect(p)
    return sqlite3.connect("dictionary.db")

def capitalize_first(s: str) -> str:
    s = s.strip()
    if not s:
        return ""
    return s[0].upper() + s[1:]

class WordLookupModal(ui.Modal, title="Tra từ điển"):
    word_input = ui.TextInput(
        label="Từ cần tra",
        placeholder="Ví dụ bác sĩ, học tập",
        min_length=2,
        max_length=50,
        required=True
    )

    async def on_submit(self, interaction: discord.Interaction):
        word = self.word_input.value.strip().lower()
        conn = get_db_connection()
        c = conn.cursor()
        c.execute("""
            SELECT COALESCE(d.definition, ''), COALESCE(d.pos, '')
            FROM words w
            JOIN word_definitions wd ON w.id = wd.word_id
            JOIN definitions d ON wd.definition_id = d.id
            WHERE w.word = ? AND w.lang_code = 'vi'
            LIMIT 5;
        """, (word,))
        rows = c.fetchall()
        conn.close()

        embed = discord.Embed(
            title=f"Tra từ {capitalize_first(word)}",
            color=discord.Color.purple()
        )
        if not rows:
            embed.description = f"Từ **{word}** chưa có định nghĩa chi tiết"
        else:
            lines = []
            for i, (defn, pos) in enumerate(rows, 1):
                pos_text = f" *({pos})*" if pos else ""
                lines.append(f"**{i}**{pos_text} {defn}")
            embed.description = "\n\n".join(lines)

        await interaction.response.send_message(embed=embed, ephemeral=True)

class ControlPanelView(ui.View):
    def __init__(self, cog, channel_id: int, guild_id: int, is_admin: bool = False):
        super().__init__(timeout=None)
        self.cog = cog
        self.channel_id = str(channel_id)
        self.guild_id = str(guild_id)
        self.is_admin = is_admin
        self.update_buttons()

    def update_buttons(self):
        self.clear_items()
        state = self.cog.get_channel_state(self.channel_id, self.guild_id)
        is_active = state.get("is_active", False)
        allow_solo = state.get("allow_solo", False)

        if self.is_admin:
            toggle_label = "Tắt kênh" if is_active else "Bật kênh"
            toggle_style = discord.ButtonStyle.danger if is_active else discord.ButtonStyle.success
            solo_label = "Tắt solo" if allow_solo else "Bật solo"

            btn_toggle = ui.Button(label=toggle_label, style=toggle_style, custom_id="py_noitu_toggle", row=0)
            btn_toggle.callback = self.btn_toggle_cb
            self.add_item(btn_toggle)

            btn_new_game = ui.Button(label="Ván mới", style=discord.ButtonStyle.primary, custom_id="py_noitu_newgame", row=0)
            btn_new_game.callback = self.btn_new_game_cb
            self.add_item(btn_new_game)

            btn_solo = ui.Button(label=solo_label, style=discord.ButtonStyle.secondary, custom_id="py_noitu_solo", row=0)
            btn_solo.callback = self.btn_solo_cb
            self.add_item(btn_solo)

            btn_lookup = ui.Button(label="Tra từ", style=discord.ButtonStyle.secondary, custom_id="py_noitu_lookup", row=0)
            btn_lookup.callback = self.btn_lookup_cb
            self.add_item(btn_lookup)

            btn_hint = ui.Button(label="Gợi ý", style=discord.ButtonStyle.secondary, custom_id="py_noitu_hint", row=1)
            btn_hint.callback = self.btn_hint_cb
            self.add_item(btn_hint)

            btn_leaderboard = ui.Button(label="Xếp hạng", style=discord.ButtonStyle.success, custom_id="py_noitu_top", row=1)
            btn_leaderboard.callback = self.btn_leaderboard_cb
            self.add_item(btn_leaderboard)

            btn_my_stats = ui.Button(label="Hồ sơ", style=discord.ButtonStyle.secondary, custom_id="py_noitu_stats", row=1)
            btn_my_stats.callback = self.btn_my_stats_cb
            self.add_item(btn_my_stats)

            btn_rules = ui.Button(label="Luật", style=discord.ButtonStyle.secondary, custom_id="py_noitu_rules", row=1)
            btn_rules.callback = self.btn_rules_cb
            self.add_item(btn_rules)
        else:
            # Thành viên thường không có quyền admin: chỉ thấy các nút chơi
            btn_lookup = ui.Button(label="Tra từ", style=discord.ButtonStyle.secondary, custom_id="py_noitu_lookup", row=0)
            btn_lookup.callback = self.btn_lookup_cb
            self.add_item(btn_lookup)

            btn_hint = ui.Button(label="Gợi ý", style=discord.ButtonStyle.secondary, custom_id="py_noitu_hint", row=0)
            btn_hint.callback = self.btn_hint_cb
            self.add_item(btn_hint)

            btn_leaderboard = ui.Button(label="Xếp hạng", style=discord.ButtonStyle.success, custom_id="py_noitu_top", row=0)
            btn_leaderboard.callback = self.btn_leaderboard_cb
            self.add_item(btn_leaderboard)

            btn_my_stats = ui.Button(label="Hồ sơ", style=discord.ButtonStyle.secondary, custom_id="py_noitu_stats", row=0)
            btn_my_stats.callback = self.btn_my_stats_cb
            self.add_item(btn_my_stats)

            btn_rules = ui.Button(label="Luật", style=discord.ButtonStyle.secondary, custom_id="py_noitu_rules", row=0)
            btn_rules.callback = self.btn_rules_cb
            self.add_item(btn_rules)

    async def btn_toggle_cb(self, interaction: discord.Interaction):
        if not interaction.user.guild_permissions.administrator:
            return await interaction.response.send_message("Bạn cần quyền Administrator", ephemeral=True)

        state = self.cog.get_channel_state(self.channel_id, self.guild_id)
        state["is_active"] = not state.get("is_active", False)
        if state["is_active"] and not state.get("current_word"):
            start_word = self.cog.get_random_start_word()
            state["current_word"] = start_word
            state["used_words"] = [start_word]
        self.cog.save_channel_state(self.channel_id, state)

        self.update_buttons()
        embed = self.cog.build_control_panel_embed(self.channel_id, self.guild_id)
        await interaction.response.edit_message(embed=embed, view=self)

        if state["is_active"]:
            parts = state["current_word"].split()
            next_syl = parts[1] if len(parts) >= 2 else ""
            await interaction.channel.send(
                f"Kênh nối từ đã được kích hoạt\nTừ mở màn **{capitalize_first(state['current_word'])}** bắt đầu bằng **{next_syl}**"
            )
        else:
            await interaction.channel.send("Kênh nối từ đã tạm dừng")

    async def btn_new_game_cb(self, interaction: discord.Interaction):
        if not interaction.user.guild_permissions.administrator:
            return await interaction.response.send_message("Bạn cần quyền Administrator", ephemeral=True)

        state = self.cog.get_channel_state(self.channel_id, self.guild_id)
        start_word = self.cog.get_random_start_word()
        state["current_word"] = start_word
        state["last_user_id"] = ""
        state["current_streak"] = 0
        state["used_words"] = [start_word]
        self.cog.save_channel_state(self.channel_id, state)

        self.update_buttons()
        embed = self.cog.build_control_panel_embed(self.channel_id, self.guild_id)
        await interaction.response.edit_message(embed=embed, view=self)

        parts = start_word.split()
        next_syl = parts[1] if len(parts) >= 2 else ""
        await interaction.channel.send(
            f"Ván mới bắt đầu với từ **{capitalize_first(start_word)}**\nBắt đầu bằng **{next_syl}**"
        )

    async def btn_solo_cb(self, interaction: discord.Interaction):
        if not interaction.user.guild_permissions.administrator:
            return await interaction.response.send_message("Bạn cần quyền Administrator", ephemeral=True)

        state = self.cog.get_channel_state(self.channel_id, self.guild_id)
        state["allow_solo"] = not state.get("allow_solo", False)
        self.cog.save_channel_state(self.channel_id, state)

        self.update_buttons()
        embed = self.cog.build_control_panel_embed(self.channel_id, self.guild_id)
        await interaction.response.edit_message(embed=embed, view=self)

    async def btn_hint_cb(self, interaction: discord.Interaction):
        state = self.cog.get_channel_state(self.channel_id, self.guild_id)
        current_word = state.get("current_word", "")
        if not current_word:
            return await interaction.response.send_message("Ván chơi chưa bắt đầu", ephemeral=True)

        parts = current_word.split()
        if len(parts) < 2:
            return await interaction.response.send_message("Từ hiện tại không hợp lệ", ephemeral=True)

        last_syl = parts[-1]
        words = self.cog.find_next_words(last_syl, 5)
        used_set = set(state.get("used_words", []))
        avail = [f"`{w}`" for w in words if w not in used_set]

        if not avail:
            msg = f"Không còn gợi ý nào cho chữ '{last_syl}'"
        else:
            msg = f"Gợi ý các từ bắt đầu bằng **{last_syl}**\n{', '.join(avail)}"
        await interaction.response.send_message(msg, ephemeral=True)

    async def btn_lookup_cb(self, interaction: discord.Interaction):
        await interaction.response.send_modal(WordLookupModal())

    async def btn_leaderboard_cb(self, interaction: discord.Interaction):
        embed = self.cog.build_leaderboard_embed(self.guild_id, interaction.guild.name)
        await interaction.response.send_message(embed=embed, ephemeral=True)

    async def btn_my_stats_cb(self, interaction: discord.Interaction):
        embed = self.cog.build_user_stat_embed(self.guild_id, interaction.user)
        await interaction.response.send_message(embed=embed, ephemeral=True)

    async def btn_rules_cb(self, interaction: discord.Interaction):
        embed = self.cog.build_rules_embed()
        await interaction.response.send_message(embed=embed, ephemeral=True)


class NoiTuCog(commands.Cog, name="Nối từ"):
    def __init__(self, bot):
        self.bot = bot
        self.channel_states = {}
        self.user_stats = {}
        self.user_violations = {}
        self.user_history = {}
        self.data_file = "wordchain_data.json"
        self.load_data()

    def load_data(self):
        if os.path.exists(self.data_file):
            try:
                with open(self.data_file, "r", encoding="utf-8") as f:
                    data = json.load(f)
                    self.channel_states = data.get("channels", {})
                    self.user_stats = data.get("stats", {})
            except Exception:
                pass

    def save_data(self):
        try:
            with open(self.data_file, "w", encoding="utf-8") as f:
                json.dump({"channels": self.channel_states, "stats": self.user_stats}, f, ensure_ascii=False, indent=2)
        except Exception:
            pass

    def get_channel_state(self, channel_id: str, guild_id: str):
        if channel_id not in self.channel_states:
            self.channel_states[channel_id] = {
                "channel_id": channel_id,
                "guild_id": guild_id,
                "is_active": False,
                "allow_solo": False,
                "current_word": "",
                "last_user_id": "",
                "current_streak": 0,
                "highest_streak": 0,
                "total_words": 0,
                "used_words": []
            }
        return self.channel_states[channel_id]

    def save_channel_state(self, channel_id: str, state: dict):
        self.channel_states[channel_id] = state
        self.save_data()

    def is_valid_word(self, word: str) -> bool:
        conn = get_db_connection()
        c = conn.cursor()
        c.execute("SELECT 1 FROM words WHERE word = ? AND lang_code = 'vi' LIMIT 1", (word.lower().strip(),))
        row = c.fetchone()
        conn.close()
        return row is not None

    def has_next_words(self, last_syllable: str) -> bool:
        conn = get_db_connection()
        c = conn.cursor()
        syl = last_syllable.lower().strip()
        c.execute("SELECT 1 FROM words WHERE word LIKE ? AND word NOT LIKE ? AND lang_code = 'vi' LIMIT 1", (syl + " %", syl + " % %"))
        row = c.fetchone()
        conn.close()
        return row is not None

    def find_next_words(self, last_syllable: str, limit: int = 5) -> list:
        conn = get_db_connection()
        c = conn.cursor()
        syl = last_syllable.lower().strip()
        c.execute("SELECT word FROM words WHERE word LIKE ? AND word NOT LIKE ? AND lang_code = 'vi' LIMIT ?", (syl + " %", syl + " % %", limit * 3))
        rows = c.fetchall()
        conn.close()
        results = []
        for r in rows:
            w = r[0]
            if len(w.split()) == 2:
                results.append(w)
                if len(results) >= limit:
                    break
        return results

    def get_random_start_word(self) -> str:
        starters = [
            "bắt đầu", "học tập", "thành công", "phát triển", "mặt trời",
            "tương lai", "hòa bình", "tự do", "hạnh phúc", "bác sĩ",
            "kinh tế", "khoa học", "công nghệ", "du lịch", "yêu thương"
        ]
        return random.choice(starters)

    def add_user_score(self, guild_id: str, user_id: str, score_gain: int, is_correct: bool, current_streak: int):
        key = f"{guild_id}_{user_id}"
        if key not in self.user_stats:
            self.user_stats[key] = {
                "guild_id": guild_id,
                "user_id": user_id,
                "score": 0,
                "words_count": 0,
                "best_streak": 0,
                "wrong_count": 0
            }
        st = self.user_stats[key]
        st["score"] += score_gain
        if is_correct:
            st["words_count"] += 1
            if current_streak > st["best_streak"]:
                st["best_streak"] = current_streak
        else:
            st["wrong_count"] += 1
        self.save_data()

    def build_control_panel_embed(self, channel_id: str, guild_id: str) -> discord.Embed:
        state = self.get_channel_state(channel_id, guild_id)
        is_active = state.get("is_active", False)
        status_text = "Đang hoạt động" if is_active else "Đang tắt"
        color = discord.Color.green() if is_active else discord.Color.red()

        current_word = state.get("current_word", "")
        formatted_word = capitalize_first(current_word) if current_word else "Chưa có"
        next_syl = "Tự do"
        if current_word:
            parts = current_word.split()
            if len(parts) >= 2:
                next_syl = parts[-1]

        streak = state.get("current_streak", 0)
        best_streak = state.get("highest_streak", 0)

        embed = discord.Embed(
            title="Bảng điều khiển nối từ",
            description=f"Kênh <#{channel_id}> ({status_text})\nTừ hiện tại **{formatted_word}**\nBắt đầu bằng **{next_syl}**\nChuỗi **{streak}** (Kỷ lục **{best_streak}**)",
            color=color
        )
        return embed

    def build_leaderboard_embed(self, guild_id: str, guild_name: str) -> discord.Embed:
        embed = discord.Embed(
            title=f"Bảng xếp hạng {guild_name}",
            color=discord.Color.gold()
        )
        guild_stats = [v for k, v in self.user_stats.items() if v.get("guild_id") == guild_id]
        guild_stats.sort(key=lambda x: (x.get("score", 0), x.get("words_count", 0)), reverse=True)
        top10 = guild_stats[:10]

        if not top10:
            embed.description = "Chưa có dữ liệu"
            return embed

        medals = ["#1", "#2", "#3"]
        lines = []
        for i, s in enumerate(top10):
            rank = medals[i] if i < 3 else f"#{i+1}"
            lines.append(f"{rank} <@{s['user_id']}> **{s.get('score', 0)}** điểm ({s.get('words_count', 0)} từ chuỗi {s.get('best_streak', 0)})")
        embed.description = "\n".join(lines)
        return embed

    def build_user_stat_embed(self, guild_id: str, user: discord.User) -> discord.Embed:
        key = f"{guild_id}_{user.id}"
        st = self.user_stats.get(key, {"score": 0, "words_count": 0, "best_streak": 0, "wrong_count": 0})
        total = st["words_count"] + st["wrong_count"]
        acc = (st["words_count"] / total * 100) if total > 0 else 0.0

        embed = discord.Embed(
            title=f"Hồ sơ {user.name}",
            color=discord.Color.blue(),
            description=f"Điểm **{st['score']}**\nĐúng **{st['words_count']}** từ\nChuỗi kỷ lục **{st['best_streak']}**\nSai **{st['wrong_count']}** lần\nChính xác **{acc:.1f}%**"
        )
        if user.avatar:
            embed.set_thumbnail(url=user.avatar.url)
        return embed

    def build_rules_embed(self) -> discord.Embed:
        return discord.Embed(
            title="Luật chơi",
            color=discord.Color.teal(),
            description="• Từ nối gồm đúng 2 tiếng\n• Âm tiết đầu từ sau trùng âm tiết cuối từ trước\n• Không lặp lại từ đã dùng\n• Không tự nối 2 từ liên tiếp khi tắt solo\n• Chat trực tiếp vào kênh để chơi\n• Từ cụt nhận thưởng lớn và mở ván mới"
        )

    @app_commands.command(name="noitu", description="Bảng điều khiển nối từ")
    async def noitu(self, interaction: discord.Interaction):
        ch_id = str(interaction.channel_id)
        g_id = str(interaction.guild_id)
        is_admin = interaction.user.guild_permissions.administrator
        state = self.get_channel_state(ch_id, g_id)
        if not state.get("is_active", False):
            active_channels = [
                cid for cid, s in self.channel_states.items()
                if s.get("guild_id") == g_id and s.get("is_active", False)
            ]
            if active_channels:
                mentions = " ".join([f"<#{cid}>" for cid in active_channels])
                msg = f"Lệnh này chỉ có thể sử dụng trong kênh {mentions}"
            else:
                msg = "Chưa có kênh nào được kích hoạt nối từ"

            view = None
            if is_admin:
                view = ui.View(timeout=None)
                btn_toggle = ui.Button(label="Kích hoạt nối từ tại kênh này", style=discord.ButtonStyle.success, custom_id="py_noitu_toggle_here")

                async def enable_cb(btn_interaction: discord.Interaction):
                    state["is_active"] = True
                    if not state.get("current_word"):
                        start_word = self.get_random_start_word()
                        state["current_word"] = start_word
                        state["used_words"] = [start_word]
                    self.save_channel_state(ch_id, state)
                    embed = self.build_control_panel_embed(ch_id, g_id)
                    ctrl_view = ControlPanelView(self, interaction.channel_id, interaction.guild_id, is_admin=True)
                    await btn_interaction.response.edit_message(content=None, embed=embed, view=ctrl_view)
                    parts = state["current_word"].split()
                    next_syl = parts[1] if len(parts) >= 2 else ""
                    await interaction.channel.send(f"Kênh nối từ đã được kích hoạt\nTừ mở màn **{capitalize_first(state['current_word'])}** bắt đầu bằng **{next_syl}**")

                btn_toggle.callback = enable_cb
                view.add_item(btn_toggle)

            return await interaction.response.send_message(msg, view=view, ephemeral=True)

        embed = self.build_control_panel_embed(ch_id, g_id)
        view = ControlPanelView(self, interaction.channel_id, interaction.guild_id, is_admin=is_admin)
        await interaction.response.send_message(embed=embed, view=view, ephemeral=True)

    @app_commands.command(name="tratu", description="Tra cứu từ điển tiếng Việt")
    @app_commands.describe(tu="Từ muốn tra cứu")
    async def tratu(self, interaction: discord.Interaction, tu: str):
        word = tu.strip().lower()
        conn = get_db_connection()
        c = conn.cursor()
        c.execute("""
            SELECT COALESCE(d.definition, ''), COALESCE(d.pos, '')
            FROM words w
            JOIN word_definitions wd ON w.id = wd.word_id
            JOIN definitions d ON wd.definition_id = d.id
            WHERE w.word = ? AND w.lang_code = 'vi'
            LIMIT 5;
        """, (word,))
        rows = c.fetchall()
        conn.close()

        embed = discord.Embed(
            title=f"Tra từ {capitalize_first(word)}",
            color=discord.Color.purple()
        )
        if not rows:
            embed.description = f"Từ **{word}** chưa có định nghĩa chi tiết"
        else:
            lines = []
            for i, (defn, pos) in enumerate(rows, 1):
                pos_text = f" *({pos})*" if pos else ""
                lines.append(f"**{i}**{pos_text} {defn}")
            embed.description = "\n\n".join(lines)

        await interaction.response.send_message(embed=embed)

    @commands.Cog.listener()
    async def on_message(self, message: discord.Message):
        if message.author.bot or not message.guild:
            return

        ch_id = str(message.channel.id)
        g_id = str(message.guild.id)
        state = self.channel_states.get(ch_id)
        if not state or not state.get("is_active"):
            return

        content = message.content.strip()
        if content.startswith("/"):
            return

        # Chống Spam & Nội Dung: Xóa link, file, media, sticker trong kênh nối từ
        if len(message.attachments) > 0 or len(message.stickers) > 0 or any(w.startswith("http://") or w.startswith("https://") or w.startswith("discord.gg/") for w in content.split()):
            try: await message.delete()
            except Exception: pass
            return

        cleaned = content.lower().strip(".,!?~`@#$%^&*()_+-=[]{}|;':\"<>/\\")
        parts = cleaned.split()

        # Lưu lịch sử tin nhắn trong kênh này
        u_key = f"{ch_id}_{message.author.id}"
        now_ts = datetime.datetime.now().timestamp()
        if u_key not in self.user_history:
            self.user_history[u_key] = []
        self.user_history[u_key] = [x for x in self.user_history[u_key] if now_ts - x["t"] <= 60.0]
        self.user_history[u_key].append({"msg": message, "t": now_ts, "c": cleaned})
        history = self.user_history[u_key]

        # 1. Anti Fast: Xóa toàn bộ tin nhắn khi gửi quá nhanh (>= 2 tin trong 1.5s)
        recent_fast = [x for x in history if now_ts - x["t"] <= 1.5]
        if len(recent_fast) >= 2:
            for item in recent_fast:
                try: await item["msg"].delete()
                except Exception: pass
            return

        # 2. Anti Dup: Xóa toàn bộ tin nhắn khi gửi trùng lặp (trong vòng 30s)
        if cleaned:
            recent_dups = [x for x in history if now_ts - x["t"] <= 30.0 and x.get("c") == cleaned]
            if len(recent_dups) >= 2:
                for item in recent_dups:
                    try: await item["msg"].delete()
                    except Exception: pass
                return

        # 3. Nếu có 1 người trả lời rồi mà mình còn trả lời tiếp: XÓA LUÔN TIN NHẮN và áp dụng phạt chống spam
        if not state.get("allow_solo") and state.get("last_user_id") == str(message.author.id):
            try: await message.delete()
            except Exception: pass

            v_key = f"{ch_id}_{message.author.id}"
            self.user_violations[v_key] = self.user_violations.get(v_key, 0) + 1
            v_count = self.user_violations[v_key]
            self.add_user_score(g_id, str(message.author.id), 0, False, 0)

            if v_count in (1, 2):
                pass
            elif v_count == 3:
                warn_msg = await message.channel.send(f"{message.author.mention} Bạn vừa trả lời lượt trước rồi, vui lòng đợi người khác nối tiếp!")
                await asyncio.sleep(5)
                try: await warn_msg.delete()
                except Exception: pass
            elif v_count == 4:
                warn_msg = await message.channel.send(f"{message.author.mention} Cảnh báo lần 2! Tiếp tục gửi khi chưa đến lượt sẽ bị tạm khóa chat!")
                await asyncio.sleep(5)
                try: await warn_msg.delete()
                except Exception: pass
            elif v_count == 5:
                try:
                    await message.author.timeout(datetime.timedelta(hours=1), reason="Spam gửi liên tiếp khi chưa đến lượt (Bước 3)")
                    mute_msg = await message.channel.send(f"Đã tạm khóa chat {message.author.mention} trong 1 giờ do spam gửi liên tiếp khi chưa đến lượt")
                    await asyncio.sleep(6)
                    try: await mute_msg.delete()
                    except Exception: pass
                except Exception: pass
            elif v_count == 6:
                try:
                    await message.author.timeout(datetime.timedelta(hours=24), reason="Vi phạm lần 4")
                    mute_msg = await message.channel.send(f"Đã tạm khóa chat {message.author.mention} trong 24 giờ do vi phạm lần 4")
                    await asyncio.sleep(6)
                    try: await mute_msg.delete()
                    except Exception: pass
                except Exception: pass
            else:
                try:
                    await message.author.timeout(datetime.timedelta(days=7), reason="Vi phạm nhiều lần")
                    mute_msg = await message.channel.send(f"Đã tạm khóa chat {message.author.mention} trong 7 ngày do vi phạm nhiều lần")
                    await asyncio.sleep(6)
                    try: await mute_msg.delete()
                    except Exception: pass
                except Exception: pass
            return

        # Kiểm tra đúng 2 tiếng: CHỈ THẢ ICON
        if len(parts) != 2:
            try: await message.add_reaction("⚠️")
            except Exception: pass
            return

        curr_word = state.get("current_word", "")
        if curr_word:
            curr_parts = curr_word.split()
            if len(curr_parts) >= 2:
                expected_prefix = curr_parts[-1]
                if parts[0] != expected_prefix:
                    try: await message.add_reaction("❌")
                    except Exception: pass
                    self.add_user_score(g_id, str(message.author.id), 0, False, 0)
                    return

        if cleaned in state.get("used_words", []):
            try: await message.add_reaction("🔁")
            except Exception: pass
            self.add_user_score(g_id, str(message.author.id), 0, False, 0)
            return

        if not self.is_valid_word(cleaned):
            try: await message.add_reaction("❌")
            except Exception: pass
            self.add_user_score(g_id, str(message.author.id), 0, False, 0)
            return

        # NỐI TỪ THÀNH CÔNG: CHỈ THẢ ICON
        try: await message.add_reaction("✅")
        except Exception: pass

        if state.get("last_user_id"):
            old_key = f"{ch_id}_{state['last_user_id']}"
            self.user_violations.pop(old_key, None)

        state["current_word"] = cleaned
        state["last_user_id"] = str(message.author.id)
        state["current_streak"] = state.get("current_streak", 0) + 1
        if state["current_streak"] > state.get("highest_streak", 0):
            state["highest_streak"] = state["current_streak"]
        state["total_words"] = state.get("total_words", 0) + 1
        state["used_words"].append(cleaned)
        self.save_channel_state(ch_id, state)

        score_gain = 10 + (state["current_streak"] // 5) * 5
        self.add_user_score(g_id, str(message.author.id), score_gain, True, state["current_streak"])

        # Mốc chuỗi đẹp
        if state["current_streak"] in [10, 25, 50, 100] or (state["current_streak"] > 100 and state["current_streak"] % 50 == 0):
            await message.channel.send(
                f"<@{message.author.id}> đạt chuỗi {state['current_streak']} từ (+{score_gain} điểm)"
            )

        # Kiểm tra từ cụt
        next_syl = parts[1]
        if not self.has_next_words(next_syl):
            bonus = 50
            self.add_user_score(g_id, str(message.author.id), bonus, True, state["current_streak"])
            await message.channel.send(
                f"<@{message.author.id}> kết thúc ván bằng từ hiểm hóc **{capitalize_first(cleaned)}** (+{bonus} điểm)\n"
                f"Chuỗi đạt {state['current_streak']} từ. Bắt đầu ván mới..."
            )
            await asyncio.sleep(2)
            start_word = self.get_random_start_word()
            state["current_word"] = start_word
            state["last_user_id"] = ""
            state["current_streak"] = 0
            state["used_words"] = [start_word]
            self.save_channel_state(ch_id, state)
            next_p = start_word.split()[1]
            await message.channel.send(
                f"Ván mới bắt đầu với từ **{capitalize_first(start_word)}**\nBắt đầu bằng **{next_p}**"
            )

async def setup(bot):
    await bot.add_cog(NoiTuCog(bot))
