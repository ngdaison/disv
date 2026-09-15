import discord
from discord.ext import commands
from discord import app_commands
from utils.data_handler import load_data, save_data, get_guild_data, get_channel_settings

class Dashboard(commands.Cog):
    def __init__(self, bot):
        self.bot = bot

    @commands.Cog.listener()
    async def on_ready(self):
        try:
            data = load_data()
            count = 0
            for gid, g_data in data.items():
                menus = g_data.get("role_menus", {})
                for msg_id, role_ids in menus.items():
                    guild = self.bot.get_guild(int(gid))
                    if guild:
                        roles = []
                        for rid in role_ids:
                            r = guild.get_role(int(rid))
                            if r: roles.append(r)
                        
                        if roles:
                            view = PersistentRoleView(roles)
                            self.bot.add_view(view, message_id=int(msg_id))
                            count += 1
            print(f"Restored {count} Role Menus.")
        except Exception as e:
            print(f"Failed to restore menus: {e}")

    @commands.hybrid_command(name="setting", description="Bảng điều khiển cài đặt Bot")
    @commands.has_permissions(administrator=True)
    async def setting(self, ctx):
        try:
            await ctx.defer(ephemeral=True)
            
            embed = discord.Embed(title="Bảng điều khiển bot", description="Bấm vào nút để bật hoặc tắt.", color=discord.Color.blue())
            embed.add_field(name="Chống spam", value=(
                "**Chặn Link**: Xoá tin nhắn chứa link.\n"
                "**Chặn Media**: Xoá Ảnh và Video.\n"
                "**Chặn File**: Xoá các file khác.\n"
                "**Chặn Chat**: Chỉ xoá tin nhắn Text (không link/file/sticker).\n"
                "**Anti Fast**: Xoá toàn bộ tin nhắn khi gửi quá nhanh qua các kênh.\n"
                "**Anti Dup**: Xoá toàn bộ tin nhắn khi gửi trùng lặp qua các kênh."
            ), inline=False)
            embed.add_field(name="Tiện ích", value=(
                "**TikTok**: Tự động tải video không logo.\n"
                "**Leveling**: Hệ thống XP và Role.\n"
                "**Sticky**: Ghim tin nhắn dưới cùng đoạn chat."
            ), inline=False)

            view = DashboardView(ctx.guild.id, ctx.channel.id)
            await ctx.send(embed=embed, view=view, ephemeral=True)
        except Exception as e:
            import traceback
            trace = traceback.format_exc()
            print(f"Error in setting command: {trace}")
            try:
                await ctx.send(f"❌ Lỗi: {e}\n```{trace[:1900]}```", ephemeral=True)
            except:
                pass

    @setting.error
    async def setting_error(self, ctx, error):
        if isinstance(error, commands.MissingPermissions):
            try:
                await ctx.reply('❌ Bạn cần quyền Administrator để sử dụng lệnh này.', ephemeral=True)
            except Exception:
                pass
            return
        raise error

class DashboardView(discord.ui.View):
    def __init__(self, guild_id, channel_id):
        super().__init__(timeout=None)
        self.guild_id = guild_id
        self.channel_id = channel_id
        self.data = load_data()
        self.settings = get_channel_settings(self.data, guild_id, channel_id)
        self.update_buttons()

    def update_buttons(self):
        # Update button styles based on state
        g_data = get_guild_data(self.data, self.guild_id)
        for child in self.children:
            if isinstance(child, discord.ui.Button):
                key = child.custom_id
                if key:
                    if key in ('as_fast', 'as_dup'):
                        state = g_data.get(key, False) or self.settings.get(key, False)
                    elif key in self.settings:
                        state = self.settings[key]
                    else:
                        continue
                    child.style = discord.ButtonStyle.green if state else discord.ButtonStyle.red

    async def toggle(self, interaction, key):
        # Toggle boolean flags (AI no longer uses webhooks)
        g_data = get_guild_data(self.data, self.guild_id)
        if key in ('as_fast', 'as_dup'):
            current_state = g_data.get(key, False) or self.settings.get(key, False)
            new_state = not current_state
            g_data[key] = new_state
            self.settings[key] = new_state
            # Sync to all channels in this guild
            for cid, ch_s in g_data.get("settings", {}).items():
                ch_s[key] = new_state
        else:
            self.settings[key] = not self.settings.get(key, False)
            
        save_data(self.data)
        self.update_buttons()
        await interaction.response.edit_message(view=self)

    # Row 1: Anti Content
    @discord.ui.button(label="Cấm Link", custom_id="ac_link", style=discord.ButtonStyle.secondary, row=0)
    async def btn_ac_link(self, interaction, button): await self.toggle(interaction, "ac_link")

    @discord.ui.button(label="Cấm Media", custom_id="ac_media", style=discord.ButtonStyle.secondary, row=0)
    async def btn_ac_media(self, interaction, button): await self.toggle(interaction, "ac_media")

    @discord.ui.button(label="Cấm File", custom_id="ac_file", style=discord.ButtonStyle.secondary, row=0)
    async def btn_ac_file(self, interaction, button): await self.toggle(interaction, "ac_file")

    @discord.ui.button(label="Cấm Chat", custom_id="ac_text", style=discord.ButtonStyle.secondary, row=0)
    async def btn_ac_text(self, interaction, button): await self.toggle(interaction, "ac_text")

    # Row 2: Anti Spam
    @discord.ui.button(label="Anti Fast", custom_id="as_fast", style=discord.ButtonStyle.secondary, row=1)
    async def btn_as_fast(self, interaction, button): await self.toggle(interaction, "as_fast")

    @discord.ui.button(label="Anti Dup", custom_id="as_dup", style=discord.ButtonStyle.secondary, row=1)
    async def btn_as_dup(self, interaction, button): await self.toggle(interaction, "as_dup")

    # Row 3: Features
    @discord.ui.button(label="Leveling", custom_id="level_enabled", style=discord.ButtonStyle.secondary, row=2)
    async def btn_level(self, interaction, button): await self.toggle(interaction, "level_enabled")

    @discord.ui.button(label="TikTok", custom_id="tiktok_enabled", style=discord.ButtonStyle.secondary, row=2)
    async def btn_tiktok(self, interaction, button): await self.toggle(interaction, "tiktok_enabled")

    @discord.ui.button(label="AI Chat", custom_id="ai_enabled", style=discord.ButtonStyle.secondary, row=2)
    async def btn_ai(self, interaction, button): await self.toggle(interaction, "ai_enabled")

    @discord.ui.button(label="Cài AI Prompt", style=discord.ButtonStyle.primary, row=3, emoji="🤖")
    async def btn_ai_prompt(self, interaction, button):
        await interaction.response.send_modal(AIPromptModal())

    # Row 4: Config
    @discord.ui.button(label="Cài Sticky", style=discord.ButtonStyle.primary, row=3, emoji="📌")
    async def btn_sticky(self, interaction, button):
        await interaction.response.send_modal(StickyModal(self.settings, self.data))

    @discord.ui.button(label="Cài Role Level", style=discord.ButtonStyle.primary, row=3, emoji="🎁")
    async def btn_role_reward(self, interaction, button):
        await interaction.response.send_modal(RoleRewardModal(self.guild_id, self.data))
        
    @discord.ui.button(label="Tạo Menu Role", style=discord.ButtonStyle.success, row=3, emoji="📋")
    async def btn_create_menu(self, interaction, button):
        await interaction.response.send_message("👇 Chọn các role bên dưới:", view=RoleSelectView(), ephemeral=True)

    @discord.ui.button(label="Cài Auto Role", style=discord.ButtonStyle.primary, row=4, emoji="⏰")
    async def btn_auto_role(self, interaction, button):
        await interaction.response.send_modal(AutoRoleModal(self.guild_id, self.data))


class StickyModal(discord.ui.Modal, title="Cài đặt Sticky Message"):
    content = discord.ui.TextInput(label="Nội dung Sticky", style=discord.TextStyle.paragraph, required=False)
    
    def __init__(self, settings, data):
        super().__init__()
        self.settings = settings
        self.data = data
        if settings.get('sticky_content'):
            self.content.default = settings['sticky_content']

    async def on_submit(self, interaction: discord.Interaction):
        self.settings['sticky_content'] = self.content.value
        self.settings['sticky_last_id'] = None 
        save_data(self.data)
        await interaction.response.send_message("✅ Đã lưu Sticky Message!", ephemeral=True)

class RoleRewardModal(discord.ui.Modal, title="Cài đặt Role Level"):
    level = discord.ui.TextInput(label="Level", placeholder="Ví dụ: 5")
    role_id = discord.ui.TextInput(label="Role ID", placeholder="Nhập ID của Role")

    def __init__(self, guild_id, data):
        super().__init__()
        self.guild_id = guild_id
        self.data = data

    async def on_submit(self, interaction: discord.Interaction):
        g = get_guild_data(self.data, self.guild_id)
        g["level_roles"][self.level.value] = self.role_id.value
        save_data(self.data)
        await interaction.response.send_message(f"✅ Đã set Role {self.role_id.value} cho Level {self.level.value}", ephemeral=True)

class AutoRoleModal(discord.ui.Modal, title="Cài đặt Auto Role"):
    role_id = discord.ui.TextInput(label="Role ID", placeholder="Nhập ID của Role")
    hours = discord.ui.TextInput(label="Thời gian (giờ)", placeholder="0 = cấp ngay, 1 = sau 1 giờ, v.v.")

    def __init__(self, guild_id, data):
        super().__init__()
        self.guild_id = guild_id
        self.data = data

    async def on_submit(self, interaction: discord.Interaction):
        try:
            role_id = int(self.role_id.value)
            hours = int(self.hours.value)
            if hours < 0:
                raise ValueError("Thời gian phải >= 0")
            g = get_guild_data(self.data, self.guild_id)
            g["auto_join_roles"][str(role_id)] = hours
            save_data(self.data)
            await interaction.response.send_message(f"✅ Đã set Auto Role {role_id} sau {hours} giờ tham gia.", ephemeral=True)
        except ValueError as e:
            await interaction.response.send_message(f"❌ Lỗi: {e}. Vui lòng nhập số hợp lệ.", ephemeral=True)


class AIPromptModal(discord.ui.Modal, title="Cấu hình AI Prompt (train.txt)"):
    prompt = discord.ui.TextInput(label="Nội dung Prompt", style=discord.TextStyle.paragraph, required=False)

    def __init__(self):
        super().__init__()
        # Load existing prompt if available
        try:
            with open('train.txt', 'r', encoding='utf-8') as f:
                content = f.read()
            self.prompt.default = content
        except Exception:
            pass

    async def on_submit(self, interaction: discord.Interaction):
        try:
            with open('train.txt', 'w', encoding='utf-8') as f:
                f.write(self.prompt.value or "")
            await interaction.response.send_message("✅ Đã lưu prompt AI (train.txt)", ephemeral=True)
        except Exception as e:
            await interaction.response.send_message(f"❌ Lỗi khi lưu prompt: {e}", ephemeral=True)

class RoleSelectView(discord.ui.View):
    @discord.ui.select(cls=discord.ui.RoleSelect, min_values=1, max_values=25, placeholder="🔍 Tìm và chọn Role...")
    async def select_roles(self, interaction: discord.Interaction, select: discord.ui.RoleSelect):
        roles = select.values
        embed = discord.Embed(title="🎭 Nhận Role", description="Chọn role bạn muốn nhận trong danh sách dưới đây:", color=discord.Color.gold())
        view = PersistentRoleView(roles)
        
        await interaction.response.send_message("⏳ Đang tạo menu...", ephemeral=True)
        msg = await interaction.channel.send(embed=embed, view=view)
        
        data = load_data()
        g_data = get_guild_data(data, interaction.guild.id)
        if "role_menus" not in g_data: g_data["role_menus"] = {}
        g_data["role_menus"][str(msg.id)] = [str(r.id) for r in roles]
        save_data(data)

class PersistentRoleView(discord.ui.View):
    def __init__(self, roles):
        super().__init__(timeout=None)
        
        # Disable duplicate filtering if needed, but select options must be unique.
        # Chunking: Discord Select Max 25.
        # If roles > 25, split into multiple select menus.
        
        chunk_size = 25
        chunks = [roles[i:i + chunk_size] for i in range(0, len(roles), chunk_size)]
        
        # Limit to 5 chunks (Discord limitation) -> 125 roles max
        if len(chunks) > 5:
            chunks = chunks[:5]

        for i, chunk in enumerate(chunks):
            options = []
            for r in chunk:
                options.append(discord.SelectOption(label=r.name, value=str(r.id), emoji="🏷️"))
            
            if options:
                # Assign a deterministic custom_id based on the chunk index
                self.add_item(RoleAssignSelect(options, custom_id=f"role_select_{i}", placeholder=f"Chọn Role (Danh sách {i+1})..."))

class RoleAssignSelect(discord.ui.Select):
    def __init__(self, options, custom_id, placeholder="Chọn role..."):
        super().__init__(placeholder=placeholder, min_values=0, max_values=len(options), options=options, custom_id=custom_id)

    async def callback(self, interaction: discord.Interaction):
        await interaction.response.defer(ephemeral=True)
        selected_ids = self.values
        all_option_ids = [o.value for o in self.options]
        
        to_remove = []
        to_add = []
        
        guild = interaction.guild
        me = guild.me
        
        for rid in all_option_ids:
            role = guild.get_role(int(rid))
            if role:
                if me.top_role <= role: continue
                if rid in selected_ids:
                    if role not in interaction.user.roles: to_add.append(role)
                else:
                    if role in interaction.user.roles: to_remove.append(role)
        
        try:
            if to_remove: await interaction.user.remove_roles(*to_remove)
            if to_add: await interaction.user.add_roles(*to_add)
            await interaction.followup.send("✅ Đã cập nhật role!", ephemeral=True)
        except Exception as e:
            await interaction.followup.send(f"❌ Lỗi: {e}", ephemeral=True)

async def setup(bot):
    await bot.add_cog(Dashboard(bot))
