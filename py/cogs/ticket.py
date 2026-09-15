import asyncio
import datetime
import html
import json
import os
import re
import shutil
import unicodedata
from typing import Any, Dict, List, Optional

import discord
from discord import app_commands
from discord.ext import commands

from utils.ticket_storage import (
    CONFIG_FILE,
    LOG_FILE,
    TICKETS_FILE,
    ensure_storage_files,
    load_config_file,
    load_logs_file,
    load_tickets_file,
    save_json,
)


ticket_group = app_commands.Group(name='ticket', description='Hệ thống ticket')

TICKET_CREATE_ID = 'ticket_create'
TICKET_CLOSE_ID = 'ticket_close'
TICKET_REOPEN_ID = 'ticket_reopen'
TICKET_DELETE_ID = 'ticket_delete'
TICKET_CLAIM_ID = 'ticket_claim'
TICKET_RENAME_ID = 'ticket_rename'
TICKET_ADD_USER_ID = 'ticket_add_user'
TICKET_REMOVE_USER_ID = 'ticket_remove_user'
TICKET_TRANSCRIPT_ID = 'ticket_transcript'

VALID_CHANNEL_NAME_PATTERN = re.compile(r'^[a-z0-9_-]{1,100}$')
MENTION_PATTERN = re.compile(r'^<@!?(\d+)>$')
USER_ID_PATTERN = re.compile(r'^(\d+)$')


def format_timestamp() -> str:
    return datetime.datetime.utcnow().replace(microsecond=0).isoformat() + 'Z'


def sanitize_channel_name(value: str) -> str:
    value = unicodedata.normalize('NFKD', value)
    value = value.encode('ascii', 'ignore').decode('ascii')
    value = value.lower().replace(' ', '-')
    value = re.sub(r'[^a-z0-9-]', '', value)
    value = re.sub(r'-{2,}', '-', value).strip('-')
    return value[:80] if value else 'ticket'


def is_admin(member: discord.Member) -> bool:
    return member.guild_permissions.administrator or member.guild_permissions.manage_guild


def build_ticket_embed(guild: Optional[discord.Guild] = None) -> discord.Embed:
    embed = discord.Embed(
        title='Support',
        description=(
            '• Chỉ tạo Hỗ Trợ nếu bạn có vấn đề về nạp tiền, bảo hành và lỗi ở trên website.\n'
            '• Vui lòng không tạo Phiếu Hỗ Trợ cho vui.\n'
            '• Tạo Phiếu Hỗ Trợ không nhắn gì sẽ Mute 24h.\n'
            '• Xin cảm ơn.'
        ),
        color=discord.Color.from_rgb(46, 204, 113),
    )
    embed.set_footer(text='TicketTool.xyz - Ticketing without clutter')
    return embed


def build_ticket_welcome_embed(member: discord.Member, ticket_number: int) -> discord.Embed:
    embed = discord.Embed(
        title=f'📩 Phiếu Hỗ Trợ #{ticket_number:04d}',
        description=(
            f'Xin chào {member.mention}!\n'
            'Vui lòng trình bày chi tiết vấn đề của bạn (kèm ảnh hoặc mã giao dịch nếu có).\n'
            'Đội ngũ hỗ trợ sẽ phản hồi trong giây lát.'
        ),
        color=discord.Color.from_rgb(46, 204, 113),
        timestamp=datetime.datetime.utcnow(),
    )
    embed.add_field(name='👤 Người tạo', value=member.mention, inline=True)
    embed.add_field(name='📌 Trạng thái', value='Đang chờ hỗ trợ', inline=True)
    embed.set_footer(text='Vui lòng đợi đội ngũ hỗ trợ phản hồi')
    return embed


def build_status_embed(title: str, description: str, colour: discord.Color) -> discord.Embed:
    embed = discord.Embed(title=title, description=description, color=colour, timestamp=datetime.datetime.utcnow())
    embed.set_footer(text='Ticket hệ thống')
    return embed


def build_log_embed(guild: discord.Guild, action: str, author: discord.Member, ticket_channel: discord.TextChannel, reason: Optional[str] = None, target: Optional[discord.abc.Snowflake] = None) -> discord.Embed:
    embed = discord.Embed(
        title='📌 Ticket Log',
        description=action,
        color=discord.Color.greyple(),
        timestamp=datetime.datetime.utcnow(),
    )
    embed.add_field(name='Server', value=f'{guild.name}', inline=False)
    embed.add_field(name='Ticket', value=f'{ticket_channel.mention} ({ticket_channel.id})', inline=False)
    embed.add_field(name='Người thực hiện', value=f'{author.mention} ({author.id})', inline=True)
    if target:
        embed.add_field(name='Mục tiêu', value=str(target), inline=True)
    if reason:
        embed.add_field(name='Lý do', value=reason, inline=False)
    embed.set_footer(text=f'Guild ID: {guild.id}')
    return embed


class TicketPanelView(discord.ui.View):
    def __init__(self, cog: 'TicketCog'):
        super().__init__(timeout=None)
        self.cog = cog

    @discord.ui.button(label='Hỗ Trợ', style=discord.ButtonStyle.danger, emoji='📩', custom_id=TICKET_CREATE_ID)
    async def create(self, interaction: discord.Interaction, button: discord.ui.Button):
        await self.cog.on_create_ticket_button(interaction)


class TicketActionView(discord.ui.View):
    def __init__(self, cog: 'TicketCog'):
        super().__init__(timeout=None)
        self.cog = cog

    @discord.ui.button(label='Đóng ticket', style=discord.ButtonStyle.danger, emoji='🔒', custom_id=TICKET_CLOSE_ID, row=0)
    async def close(self, interaction: discord.Interaction, button: discord.ui.Button):
        await self.cog.on_close_ticket_button(interaction)

    @discord.ui.button(label='Claim ticket', style=discord.ButtonStyle.secondary, emoji='🟡', custom_id=TICKET_CLAIM_ID, row=0)
    async def claim(self, interaction: discord.Interaction, button: discord.ui.Button):
        await self.cog.on_claim_ticket_button(interaction)

    @discord.ui.button(label='Transcript', style=discord.ButtonStyle.primary, emoji='📄', custom_id=TICKET_TRANSCRIPT_ID, row=0)
    async def transcript(self, interaction: discord.Interaction, button: discord.ui.Button):
        await self.cog.on_transcript_button(interaction)

    @discord.ui.button(label='Thêm người', style=discord.ButtonStyle.success, emoji='➕', custom_id=TICKET_ADD_USER_ID, row=1)
    async def add_user(self, interaction: discord.Interaction, button: discord.ui.Button):
        await self.cog.on_add_user_button(interaction)

    @discord.ui.button(label='Xóa người', style=discord.ButtonStyle.secondary, emoji='➖', custom_id=TICKET_REMOVE_USER_ID, row=1)
    async def remove_user(self, interaction: discord.Interaction, button: discord.ui.Button):
        await self.cog.on_remove_user_button(interaction)

    @discord.ui.button(label='Đổi tên', style=discord.ButtonStyle.secondary, emoji='✏️', custom_id=TICKET_RENAME_ID, row=1)
    async def rename(self, interaction: discord.Interaction, button: discord.ui.Button):
        await self.cog.on_rename_ticket_button(interaction)

    @discord.ui.button(label='Xóa ticket', style=discord.ButtonStyle.danger, emoji='🗑️', custom_id=TICKET_DELETE_ID, row=1)
    async def delete(self, interaction: discord.Interaction, button: discord.ui.Button):
        await self.cog.on_delete_ticket_button(interaction)


class CloseTicketModal(discord.ui.Modal, title='Lý do đóng ticket'):
    reason = discord.ui.TextInput(label='Lý do', style=discord.TextStyle.long, required=False, max_length=250)

    def __init__(self, cog: 'TicketCog', channel_id: int):
        super().__init__()
        self.cog = cog
        self.channel_id = channel_id

    async def on_submit(self, interaction: discord.Interaction) -> None:
        await self.cog.close_ticket(interaction, self.channel_id, self.reason.value.strip() or 'Không có lý do')


class RenameTicketModal(discord.ui.Modal, title='Đổi tên ticket'):
    new_name = discord.ui.TextInput(label='Tên mới', style=discord.TextStyle.short, required=True, max_length=80)

    def __init__(self, cog: 'TicketCog', channel_id: int):
        super().__init__()
        self.cog = cog
        self.channel_id = channel_id

    async def on_submit(self, interaction: discord.Interaction) -> None:
        await self.cog.rename_ticket(interaction, self.channel_id, self.new_name.value)


class AddRemoveUserModal(discord.ui.Modal):
    user_input = discord.ui.TextInput(label='ID hoặc mention người dùng', style=discord.TextStyle.short, required=True, max_length=100)

    def __init__(self, cog: 'TicketCog', channel_id: int, action: str):
        title = 'Thêm người vào ticket' if action == 'add' else 'Xóa người khỏi ticket'
        super().__init__(title=title)
        self.cog = cog
        self.channel_id = channel_id
        self.action = action

    async def on_submit(self, interaction: discord.Interaction) -> None:
        if self.action == 'add':
            await self.cog.modify_ticket_user(interaction, self.channel_id, self.user_input.value, add=True)
        else:
            await self.cog.modify_ticket_user(interaction, self.channel_id, self.user_input.value, add=False)


class TicketCog(commands.Cog):
    def __init__(self, bot: commands.Bot):
        self.bot = bot
        self.lock = asyncio.Lock()
        self.config_data: Dict[str, Any] = {}
        self.ticket_data: Dict[str, Any] = {}
        self.log_data: Dict[str, Any] = {}
        self.click_cooldowns: Dict[int, float] = {}

    async def cog_load(self) -> None:
        await ensure_storage_files()
        await self.reload_storage()
        try:
            self.bot.tree.add_command(ticket_group, override=True)
        except Exception as e:
            print(f'Failed to register ticket command group: {e}')
        self.bot.add_view(TicketPanelView(self))
        self.bot.add_view(TicketActionView(self))

    async def reload_storage(self) -> None:
        async with self.lock:
            self.config_data = await load_config_file()
            self.ticket_data = await load_tickets_file()
            self.log_data = await load_logs_file()

    async def write_config(self) -> None:
        async with self.lock:
            await save_json(CONFIG_FILE, self.config_data)

    async def write_tickets(self) -> None:
        async with self.lock:
            await save_json(TICKETS_FILE, self.ticket_data)

    async def write_logs(self) -> None:
        async with self.lock:
            await save_json(LOG_FILE, self.log_data)

    def get_guild_config(self, guild_id: int) -> Optional[Dict[str, Any]]:
        return self.config_data.get('guilds', {}).get(str(guild_id))

    def get_ticket(self, channel_id: int) -> Optional[Dict[str, Any]]:
        return self.ticket_data.get('tickets', {}).get(str(channel_id))

    def parse_user_string(self, guild: discord.Guild, value: str) -> Optional[discord.Member]:
        value = value.strip()
        match = MENTION_PATTERN.match(value)
        if match:
            return guild.get_member(int(match.group(1)))
        match = USER_ID_PATTERN.match(value)
        if match:
            return guild.get_member(int(match.group(1)))
        if value.isdigit():
            return guild.get_member(int(value))
        return None

    def make_ticket_search(self, guild_id: int, owner_id: int) -> Optional[Dict[str, Any]]:
        for ticket in self.ticket_data.get('tickets', {}).values():
            if ticket.get('guild_id') == guild_id and ticket.get('owner_id') == owner_id and ticket.get('status') in ('open', 'claimed'):
                return ticket
        return None

    def format_ticket_record(self, ticket: Dict[str, Any]) -> str:
        status = ticket.get('status', 'unknown')
        number = ticket.get('ticket_number', 0)
        owner = ticket.get('owner_name', 'Unknown')
        channel_id = ticket.get('channel_id')
        ticket_channel = self.bot.get_channel(channel_id)
        channel_mention = ticket_channel.mention if ticket_channel else f'<#{channel_id}>'
        return f'`#{number:03d}` {channel_mention} • {owner} • `{status}`'

    async def log_ticket_action(self, guild: discord.Guild, ticket_channel: Optional[discord.TextChannel], action: str, user: discord.Member, target_id: Optional[int] = None, reason: Optional[str] = None) -> None:
        if not ticket_channel:
            return
        entry = {
            'guild_id': guild.id,
            'ticket_channel_id': ticket_channel.id,
            'action': action,
            'user_id': user.id,
            'target_id': target_id,
            'reason': reason,
            'created_at': format_timestamp(),
        }
        self.log_data.setdefault('logs', []).append(entry)
        await self.write_logs()

        config = self.get_guild_config(guild.id)
        if not config:
            return
        log_channel = guild.get_channel(config.get('log_channel_id'))
        if not log_channel:
            return
        embed = build_log_embed(guild, action, user, ticket_channel, reason=reason, target=target_id)
        try:
            await log_channel.send(embed=embed)
        except discord.Forbidden:
            pass
        except Exception:
            pass

    async def ensure_guild_config(self, guild_id: int) -> Dict[str, Any]:
        if 'guilds' not in self.config_data:
            self.config_data['guilds'] = {}
        guild_key = str(guild_id)
        if guild_key not in self.config_data['guilds']:
            self.config_data['guilds'][guild_key] = {
                'panel_channel_id': 0,
                'panel_message_id': 0,
                'ticket_category_id': 0,
                'staff_role_id': 0,
                'log_channel_id': 0,
                'ticket_count': 0,
                'created_at': format_timestamp(),
                'updated_at': format_timestamp(),
            }
        return self.config_data['guilds'][guild_key]

    async def is_staff_or_admin(self, member: discord.Member, config: Optional[Dict[str, Any]]) -> bool:
        if is_admin(member):
            return True
        if config and config.get('staff_role_id'):
            return any(role.id == config.get('staff_role_id') for role in member.roles)
        return False

    async def send_error(self, interaction: discord.Interaction, content: str, ephemeral: bool = True) -> None:
        if interaction.response.is_done():
            await interaction.followup.send(content, ephemeral=ephemeral)
        else:
            await interaction.response.send_message(content, ephemeral=ephemeral)

    async def send_success(self, interaction: discord.Interaction, content: str, ephemeral: bool = True) -> None:
        await self.send_error(interaction, content, ephemeral=ephemeral)

    async def build_ticket_channel(self, guild: discord.Guild, owner: discord.Member, config: Dict[str, Any]) -> Optional[discord.TextChannel]:
        category_id = config.get('ticket_category_id')
        category = guild.get_channel(category_id) if category_id else None
        
        # Nếu chưa có Category hoặc ID không hợp lệ, tự tìm hoặc tạo Category "🎟️ HỖ TRỢ"
        if not isinstance(category, discord.CategoryChannel):
            category = discord.utils.get(guild.categories, name='🎟️ HỖ TRỢ') or discord.utils.get(guild.categories, name='HỖ TRỢ') or discord.utils.get(guild.categories, name='Tickets')
            if not category:
                try:
                    category = await guild.create_category(name='🎟️ HỖ TRỢ', reason='Tự động tạo danh mục cho ticket')
                    config['ticket_category_id'] = category.id
                    await self.write_config()
                except Exception as e:
                    print(f"Cannot create category: {e}")
                    category = None

        ticket_number = config.get('ticket_count', 0) + 1
        safe_number = f'{ticket_number:04d}'
        channel_name = f'ho-tro-{safe_number}'

        overwrites = {
            guild.default_role: discord.PermissionOverwrite(view_channel=False),
            owner: discord.PermissionOverwrite(view_channel=True, send_messages=True, read_message_history=True, attach_files=True, embed_links=True, add_reactions=True),
        }
        staff_role_id = config.get('staff_role_id')
        staff_role = guild.get_role(staff_role_id) if staff_role_id else None
        if staff_role:
            overwrites[staff_role] = discord.PermissionOverwrite(view_channel=True, send_messages=True, read_message_history=True, manage_messages=True, attach_files=True, embed_links=True)
        bot_member = guild.me
        if bot_member:
            overwrites[bot_member] = discord.PermissionOverwrite(view_channel=True, send_messages=True, manage_messages=True, manage_channels=True, embed_links=True, attach_files=True, read_message_history=True)

        channel = await guild.create_text_channel(
            name=channel_name,
            category=category,
            overwrites=overwrites,
            reason=f'Ticket created by {owner} ({owner.id})',
        )
        return channel

    async def save_ticket_record(self, channel: discord.TextChannel, owner: discord.Member, config: Dict[str, Any]) -> Dict[str, Any]:
        ticket_number = config.get('ticket_count', 0) + 1
        config['ticket_count'] = ticket_number
        config['updated_at'] = format_timestamp()
        ticket = {
            'ticket_id': f'{channel.id}',
            'guild_id': channel.guild.id,
            'channel_id': channel.id,
            'owner_id': owner.id,
            'owner_name': str(owner),
            'status': 'open',
            'claimed_by': None,
            'ticket_number': ticket_number,
            'channel_name': channel.name,
            'created_at': format_timestamp(),
            'closed_at': None,
            'closed_by': None,
            'close_reason': None,
            'added_users': [],
            'removed_users': [],
            'transcript_file': None,
            'messages_count': 0,
        }
        self.ticket_data.setdefault('tickets', {})[str(channel.id)] = ticket
        await self.write_tickets()
        await self.write_config()
        return ticket

    async def render_ticket_panel(self, channel: discord.TextChannel, guild: discord.Guild) -> discord.Message:
        embed = build_ticket_embed(guild)
        view = TicketPanelView(self)
        return await channel.send(embed=embed, view=view)

    async def on_create_ticket_button(self, interaction: discord.Interaction) -> None:
        guild = interaction.guild
        author = interaction.user
        if not guild or not isinstance(author, discord.Member):
            return

        if interaction.user.id in self.click_cooldowns and datetime.datetime.utcnow().timestamp() - self.click_cooldowns[interaction.user.id] < 5:
            return await self.send_error(interaction, 'Vui lòng chờ vài giây trước khi tạo ticket tiếp theo.', ephemeral=True)
        self.click_cooldowns[interaction.user.id] = datetime.datetime.utcnow().timestamp()

        config = await self.ensure_guild_config(guild.id)

        if self.make_ticket_search(guild.id, author.id):
            active = self.make_ticket_search(guild.id, author.id)
            channel = guild.get_channel(active.get('channel_id'))
            if channel:
                return await self.send_error(interaction, f'Bạn đã có ticket đang mở: {channel.mention}', ephemeral=True)
            active['status'] = 'deleted'
            await self.write_tickets()

        async with self.lock:
            ticket_channel = await self.build_ticket_channel(guild, author, config)
            if not ticket_channel:
                return await self.send_error(interaction, 'Không thể tạo kênh ticket. Vui lòng kiểm tra quyền bot quản lý kênh.', ephemeral=True)

            ticket = await self.save_ticket_record(ticket_channel, author, config)

        welcome_embed = build_ticket_welcome_embed(author, ticket.get('ticket_number', 0))
        staff_role_id = config.get("staff_role_id")
        staff_ping = f'<@&{staff_role_id}>' if staff_role_id else ''
        content_ping = f'{author.mention} {staff_ping}'.strip()
        await ticket_channel.send(content=content_ping, embed=welcome_embed, view=TicketActionView(self))
        await self.log_ticket_action(guild, ticket_channel, 'Tạo ticket mới', author)

        await interaction.response.send_message(f'✅ Ticket của bạn đã được tạo: {ticket_channel.mention}', ephemeral=True)

    async def fetch_ticket_channel(self, channel: Optional[discord.abc.Snowflake]) -> Optional[discord.TextChannel]:
        if not channel:
            return None
        if isinstance(channel, discord.TextChannel):
            return channel
        if isinstance(channel, discord.abc.Snowflake):
            return self.bot.get_channel(channel.id)
        return None

    async def close_ticket(self, interaction: discord.Interaction, channel_id: int, reason: str) -> None:
        guild = interaction.guild
        author = interaction.user
        if not guild or not isinstance(author, discord.Member):
            return

        ticket = self.get_ticket(channel_id)
        if not ticket:
            return await self.send_error(interaction, 'Không tìm thấy ticket trong dữ liệu.', ephemeral=True)

        if ticket.get('status') not in ('open', 'claimed'):
            return await self.send_error(interaction, 'Ticket này hiện không thể đóng vì không ở trạng thái mở.', ephemeral=True)

        if not await self.is_staff_or_admin(author, self.get_guild_config(guild.id)) and ticket.get('owner_id') != author.id:
            return await self.send_error(interaction, 'Bạn không có quyền đóng ticket này.', ephemeral=True)

        channel = guild.get_channel(channel_id)
        if not channel:
            ticket['status'] = 'deleted'
            ticket['closed_at'] = format_timestamp()
            ticket['closed_by'] = author.id
            ticket['close_reason'] = reason
            await self.write_tickets()
            return await self.send_error(interaction, 'Kênh ticket bị xóa thủ công. Dữ liệu đã được cập nhật.', ephemeral=True)

        ticket['status'] = 'closed'
        ticket['closed_at'] = format_timestamp()
        ticket['closed_by'] = author.id
        ticket['close_reason'] = reason
        ticket['channel_name'] = channel.name
        await self.write_tickets()

        owner_member = guild.get_member(ticket['owner_id'])
        if owner_member:
            try:
                await channel.set_permissions(owner_member, send_messages=False, reason='Khóa quyền gửi của owner khi đóng ticket')
            except discord.HTTPException:
                pass

        close_embed = build_status_embed('🔒 Ticket đã đóng', f'Ticket này đã bị đóng bởi {author.mention}.\nLý do: {reason}', discord.Color.red())
        await channel.send(embed=close_embed, view=TicketActionView(self))
        await self.log_ticket_action(guild, channel, 'Đóng ticket', author, reason=reason)
        await self.send_success(interaction, '✅ Ticket đã được đóng.', ephemeral=True)

    async def reopen_ticket(self, interaction: discord.Interaction, channel_id: int) -> None:
        guild = interaction.guild
        author = interaction.user
        if not guild or not isinstance(author, discord.Member):
            return

        ticket = self.get_ticket(channel_id)
        if not ticket:
            return await self.send_error(interaction, 'Không tìm thấy ticket.', ephemeral=True)

        if ticket.get('status') != 'closed':
            return await self.send_error(interaction, 'Ticket này chỉ có thể mở lại khi đang ở trạng thái đóng.', ephemeral=True)

        if not await self.is_staff_or_admin(author, self.get_guild_config(guild.id)):
            return await self.send_error(interaction, 'Chỉ staff hoặc admin mới được mở lại ticket.', ephemeral=True)

        channel = guild.get_channel(channel_id)
        if not channel:
            ticket['status'] = 'deleted'
            await self.write_tickets()
            return await self.send_error(interaction, 'Kênh ticket đã bị xóa thủ công.', ephemeral=True)

        ticket['status'] = 'open'
        ticket['closed_at'] = None
        ticket['closed_by'] = None
        ticket['close_reason'] = None
        await self.write_tickets()

        original_name = ticket.get('channel_name', channel.name).lstrip('closed-')
        safe_name = sanitize_channel_name(original_name)[:80]
        try:
            await channel.edit(name=safe_name, reason='Mở lại ticket')
        except discord.Forbidden:
            pass

        owner_member = guild.get_member(ticket['owner_id'])
        if owner_member:
            await channel.set_permissions(owner_member, send_messages=True, reason='Mở quyền gửi cho owner khi mở lại ticket')
        reopen_embed = build_status_embed('🔓 Ticket đã mở lại', f'Veui lòng tiếp tục trao đổi tại kênh này.', discord.Color.green())
        await channel.send(embed=reopen_embed, view=TicketActionView(self))
        await self.log_ticket_action(guild, channel, 'Mở lại ticket', author)
        await self.send_success(interaction, '✅ Ticket đã được mở lại.', ephemeral=True)

    async def delete_ticket(self, interaction: discord.Interaction, channel_id: int) -> None:
        guild = interaction.guild
        author = interaction.user
        if not guild or not isinstance(author, discord.Member):
            return

        ticket = self.get_ticket(channel_id)
        if not ticket:
            return await self.send_error(interaction, 'Không tìm thấy ticket trong dữ liệu.', ephemeral=True)

        if not await self.is_staff_or_admin(author, self.get_guild_config(guild.id)):
            return await self.send_error(interaction, 'Bạn không có quyền xóa ticket này.', ephemeral=True)

        channel = guild.get_channel(channel_id)
        if channel:
            transcript_file = await self.generate_transcript(channel, ticket)
            await self.send_transcript_to_log(guild, channel, transcript_file, author)
            await asyncio.sleep(3)
            try:
                await channel.delete(reason='Xóa ticket')
            except discord.Forbidden:
                return await self.send_error(interaction, 'Bot thiếu quyền xóa kênh ticket.', ephemeral=True)
            except Exception:
                pass

        ticket['status'] = 'deleted'
        ticket['closed_at'] = format_timestamp()
        ticket['closed_by'] = author.id
        ticket['close_reason'] = 'Xóa ticket'
        await self.write_tickets()
        await self.log_ticket_action(guild, channel if channel else None, 'Xóa ticket', author)

        await self.send_success(interaction, '✅ Ticket đã được xóa và transcript đã được gửi tới kênh log.', ephemeral=True)

    async def claim_ticket(self, interaction: discord.Interaction, channel_id: int) -> None:
        guild = interaction.guild
        author = interaction.user
        if not guild or not isinstance(author, discord.Member):
            return

        ticket = self.get_ticket(channel_id)
        if not ticket:
            return await self.send_error(interaction, 'Không tìm thấy ticket.', ephemeral=True)

        if ticket.get('status') == 'deleted':
            return await self.send_error(interaction, 'Ticket này đã bị xóa.', ephemeral=True)

        if not await self.is_staff_or_admin(author, self.get_guild_config(guild.id)):
            return await self.send_error(interaction, 'Chỉ staff hoặc admin mới được claim ticket.', ephemeral=True)

        current_claim = ticket.get('claimed_by')
        if current_claim and current_claim != author.id and not is_admin(author):
            claimer = guild.get_member(current_claim)
            claimer_name = str(claimer) if claimer else str(current_claim)
            return await self.send_error(interaction, f'Ticket đang được xử lý bởi {claimer_name}.', ephemeral=True)

        ticket['status'] = 'claimed'
        ticket['claimed_by'] = author.id
        await self.write_tickets()

        channel = guild.get_channel(channel_id)
        if channel:
            embed = build_status_embed('🟡 Ticket đang được xử lý', f'{author.mention} đã nhận ticket.', discord.Color.gold())
            await channel.send(embed=embed, view=TicketActionView(self))
        await self.log_ticket_action(guild, channel if channel else None, 'Claim ticket', author)
        await self.send_success(interaction, '✅ Bạn đã claim ticket này.', ephemeral=True)

    async def rename_ticket(self, interaction: discord.Interaction, channel_id: int, new_name: str) -> None:
        guild = interaction.guild
        author = interaction.user
        if not guild or not isinstance(author, discord.Member):
            return

        ticket = self.get_ticket(channel_id)
        if not ticket:
            return await self.send_error(interaction, 'Không tìm thấy ticket.', ephemeral=True)

        if not await self.is_staff_or_admin(author, self.get_guild_config(guild.id)):
            return await self.send_error(interaction, 'Chỉ staff hoặc admin mới được đổi tên ticket.', ephemeral=True)

        safe_name = sanitize_channel_name(new_name)
        if not VALID_CHANNEL_NAME_PATTERN.match(safe_name):
            return await self.send_error(interaction, 'Tên kênh không hợp lệ. Vui lòng chỉ dùng chữ thường, số, gạch ngang hoặc gạch dưới.', ephemeral=True)

        channel = guild.get_channel(channel_id)
        if not channel:
            return await self.send_error(interaction, 'Kênh ticket đã bị xóa.', ephemeral=True)

        try:
            await channel.edit(name=safe_name, reason=f'Đổi tên ticket bởi {author}')
        except discord.Forbidden:
            return await self.send_error(interaction, 'Bot thiếu quyền đổi tên kênh.', ephemeral=True)

        ticket['channel_name'] = safe_name
        await self.write_tickets()
        await self.log_ticket_action(guild, channel, 'Đổi tên ticket', author, reason=safe_name)
        await self.send_success(interaction, f'✅ Đã đổi tên ticket thành `{safe_name}`.', ephemeral=True)

    async def modify_ticket_user(self, interaction: discord.Interaction, channel_id: int, user_value: str, add: bool) -> None:
        guild = interaction.guild
        author = interaction.user
        if not guild or not isinstance(author, discord.Member):
            return

        ticket = self.get_ticket(channel_id)
        if not ticket:
            return await self.send_error(interaction, 'Không tìm thấy ticket.', ephemeral=True)

        if not await self.is_staff_or_admin(author, self.get_guild_config(guild.id)):
            return await self.send_error(interaction, 'Chỉ staff hoặc admin mới có quyền này.', ephemeral=True)

        member = self.parse_user_string(guild, user_value)
        if not member:
            return await self.send_error(interaction, 'Không tìm thấy người dùng trong server.', ephemeral=True)

        if member.id == ticket.get('owner_id'):
            return await self.send_error(interaction, 'Không thể thêm hoặc xóa owner ticket.', ephemeral=True)

        staff_role_id = self.get_guild_config(guild.id).get('staff_role_id')
        if member.id == staff_role_id:
            return await self.send_error(interaction, 'Không thể thay đổi quyền cho staff role bằng thao tác này.', ephemeral=True)

        channel = guild.get_channel(channel_id)
        if not channel:
            return await self.send_error(interaction, 'Kênh ticket đã bị xóa.', ephemeral=True)

        if add:
            await channel.set_permissions(member, view_channel=True, send_messages=True, read_message_history=True, attach_files=True, embed_links=True)
            ticket.setdefault('added_users', [])
            if member.id not in ticket['added_users']:
                ticket['added_users'].append(member.id)
            await self.send_success(interaction, f'✅ Đã thêm {member.mention} vào ticket.', ephemeral=True)
            await channel.send(embed=build_status_embed('➕ Người dùng được thêm', f'{member.mention} đã được thêm vào ticket.', discord.Color.green()))
            await self.log_ticket_action(guild, channel, 'Thêm người vào ticket', author, target_id=member.id)
        else:
            await channel.set_permissions(member, overwrite=None)
            ticket.setdefault('removed_users', [])
            if member.id not in ticket['removed_users']:
                ticket['removed_users'].append(member.id)
            await self.send_success(interaction, f'✅ Đã gỡ {member.mention} khỏi ticket.', ephemeral=True)
            await channel.send(embed=build_status_embed('➖ Người dùng bị xóa', f'{member.mention} đã bị gỡ khỏi ticket.', discord.Color.orange()))
            await self.log_ticket_action(guild, channel, 'Xóa người khỏi ticket', author, target_id=member.id)

        await self.write_tickets()

    async def generate_transcript(self, channel: discord.TextChannel, ticket: Dict[str, Any]) -> str:
        os.makedirs('transcripts', exist_ok=True)
        safe_name = sanitize_channel_name(channel.name) or str(channel.id)
        timestamp = datetime.datetime.utcnow().strftime('%Y%m%d-%H%M%S')
        filename = f'transcripts/{safe_name}-{timestamp}.html'
        messages: List[discord.Message] = []
        async for msg in channel.history(limit=None, oldest_first=True):
            messages.append(msg)

        lines: List[str] = []
        lines.append('<!DOCTYPE html><html lang="vi"><head><meta charset="utf-8"><title>Transcript</title></head><body>')
        lines.append(f'<h1>Transcript {html.escape(channel.name)}</h1>')
        lines.append(f'<p>Server: {html.escape(channel.guild.name)} ({channel.guild.id})</p>')
        lines.append(f'<p>Ticket owner: {html.escape(ticket.get("owner_name", "Unknown"))} ({ticket.get("owner_id")})</p>')
        lines.append(f'<p>Ticket ID: {ticket.get("ticket_id")}</p>')
        lines.append(f'<p>Created at: {ticket.get("created_at")}</p>')
        if ticket.get('closed_at'):
            lines.append(f'<p>Closed at: {ticket.get("closed_at")}</p>')
        lines.append('<hr>')
        for msg in messages:
            author = html.escape(str(msg.author))
            content = html.escape(msg.content or '')
            created = msg.created_at.isoformat()
            lines.append(f'<div><strong>{author}</strong> <small>{created} ({msg.author.id})</small><br/>')
            if content:
                lines.append(f'<p>{content}</p>')
            if msg.embeds:
                for embed in msg.embeds:
                    lines.append(f'<p><em>Embed:</em> {html.escape(str(embed.to_dict()))}</p>')
            if msg.attachments:
                for attachment in msg.attachments:
                    lines.append(f'<p><em>Attachment:</em> <a href="{html.escape(attachment.url)}">{html.escape(attachment.filename)}</a></p>')
            if msg.stickers:
                for sticker in msg.stickers:
                    lines.append(f'<p><em>Sticker:</em> {html.escape(sticker.name)}</p>')
            lines.append('</div><hr>')
        lines.append('</body></html>')

        with open(filename, 'w', encoding='utf-8') as f:
            f.write('\n'.join(lines))

        ticket['transcript_file'] = filename
        ticket['messages_count'] = len(messages)
        await self.write_tickets()
        return filename

    async def send_transcript_to_log(self, guild: discord.Guild, channel: discord.TextChannel, transcript_file: str, author: discord.Member) -> None:
        config = self.get_guild_config(guild.id)
        if not config:
            return
        log_channel = guild.get_channel(config.get('log_channel_id'))
        if not log_channel:
            return
        try:
            await log_channel.send(content=f'📄 Transcript cho {channel.mention}', file=discord.File(transcript_file))
        except discord.Forbidden:
            pass
        except Exception:
            pass
        await self.log_ticket_action(guild, channel, 'Transcript ticket', author)

    async def on_close_ticket_button(self, interaction: discord.Interaction) -> None:
        if interaction.channel is None:
            return
        ticket = self.get_ticket(interaction.channel.id)
        if not ticket:
            return await self.send_error(interaction, 'Đây không phải là kênh ticket hợp lệ.', ephemeral=True)
        await interaction.response.send_modal(CloseTicketModal(self, interaction.channel.id))

    async def on_reopen_ticket_button(self, interaction: discord.Interaction) -> None:
        if interaction.channel is None:
            return
        await self.reopen_ticket(interaction, interaction.channel.id)

    async def on_delete_ticket_button(self, interaction: discord.Interaction) -> None:
        if interaction.channel is None:
            return
        await self.delete_ticket(interaction, interaction.channel.id)

    async def on_claim_ticket_button(self, interaction: discord.Interaction) -> None:
        if interaction.channel is None:
            return
        await self.claim_ticket(interaction, interaction.channel.id)

    async def on_rename_ticket_button(self, interaction: discord.Interaction) -> None:
        if interaction.channel is None:
            return
        await interaction.response.send_modal(RenameTicketModal(self, interaction.channel.id))

    async def on_add_user_button(self, interaction: discord.Interaction) -> None:
        if interaction.channel is None:
            return
        await interaction.response.send_modal(AddRemoveUserModal(self, interaction.channel.id, action='add'))

    async def on_remove_user_button(self, interaction: discord.Interaction) -> None:
        if interaction.channel is None:
            return
        await interaction.response.send_modal(AddRemoveUserModal(self, interaction.channel.id, action='remove'))

    async def on_transcript_button(self, interaction: discord.Interaction) -> None:
        if interaction.channel is None:
            return
        ticket = self.get_ticket(interaction.channel.id)
        if not ticket:
            return await self.send_error(interaction, 'Đây không phải là kênh ticket hợp lệ.', ephemeral=True)
        channel = interaction.channel
        if not isinstance(channel, discord.TextChannel):
            return await self.send_error(interaction, 'Không thể tạo transcript ở vị trí này.', ephemeral=True)
        await interaction.response.defer(ephemeral=True)
        transcript_file = await self.generate_transcript(channel, ticket)
        await self.send_transcript_to_log(channel.guild, channel, transcript_file, interaction.user)
        await self.send_success(interaction, '✅ Transcript đã được tạo và gửi vào kênh log.', ephemeral=True)

    async def response_with_embed(self, interaction: discord.Interaction, embed: discord.Embed, ephemeral: bool = True) -> None:
        if interaction.response.is_done():
            await interaction.followup.send(embed=embed, ephemeral=ephemeral)
        else:
            await interaction.response.send_message(embed=embed, ephemeral=ephemeral)

    @app_commands.guild_only()
    @app_commands.default_permissions(administrator=True)
    @ticket_group.command(name='setup', description='Cấu hình hệ thống ticket')
    async def ticket_setup(self, interaction: discord.Interaction, panel_channel: discord.TextChannel, ticket_category: discord.CategoryChannel, staff_role: discord.Role, log_channel: discord.TextChannel) -> None:
        guild = interaction.guild
        author = interaction.user
        if not guild or not isinstance(author, discord.Member):
            return
        config = await self.ensure_guild_config(guild.id)
        config['panel_channel_id'] = panel_channel.id
        config['ticket_category_id'] = ticket_category.id
        config['staff_role_id'] = staff_role.id
        config['log_channel_id'] = log_channel.id
        config['updated_at'] = format_timestamp()
        if not config.get('created_at'):
            config['created_at'] = format_timestamp()

        await self.write_config()

        try:
            panel_message = await self.render_ticket_panel(panel_channel, guild)
            config['panel_message_id'] = panel_message.id
            await self.write_config()
        except discord.Forbidden:
            return await self.send_error(interaction, 'Bot thiếu quyền gửi tin nhắn ở kênh panel.', ephemeral=True)
        except Exception as exc:
            return await self.send_error(interaction, f'Không thể tạo panel ticket: {exc}', ephemeral=True)

        embed = discord.Embed(
            title='🎉 Cấu hình Ticket thành công',
            description=(
                f'• **Kênh panel:** {panel_channel.mention}\n'
                f'• **Danh mục ticket:** {ticket_category.name}\n'
                f'• **Role hỗ trợ:** {staff_role.mention}\n'
                f'• **Kênh log:** {log_channel.mention}'
            ),
            color=0x2ecc71
        )
        await self.response_with_embed(interaction, embed, ephemeral=True)

    @app_commands.guild_only()
    @app_commands.default_permissions(administrator=True)
    @ticket_group.command(name='send', description='Gửi bảng tạo ticket vào kênh chỉ định (mẫu Support chuẩn)')
    @app_commands.describe(channel='Kênh muốn gửi bảng ticket (mặc định là kênh hiện tại)')
    async def ticket_send(self, interaction: discord.Interaction, channel: Optional[discord.TextChannel] = None) -> None:
        guild = interaction.guild
        if not guild or not isinstance(interaction.user, discord.Member):
            return
        target_channel = channel or interaction.channel
        if not isinstance(target_channel, discord.TextChannel):
            return await self.send_error(interaction, 'Vui lòng chọn một kênh text hợp lệ.', ephemeral=True)
        
        config = await self.ensure_guild_config(guild.id)
        config['panel_channel_id'] = target_channel.id
        await self.write_config()

        try:
            panel_message = await self.render_ticket_panel(target_channel, guild)
            config['panel_message_id'] = panel_message.id
            await self.write_config()
            await self.send_success(interaction, f'✅ Đã gửi bảng Hỗ Trợ vào kênh {target_channel.mention}', ephemeral=True)
        except discord.Forbidden:
            await self.send_error(interaction, 'Bot thiếu quyền gửi tin nhắn hoặc embed ở kênh đó.', ephemeral=True)
        except Exception as e:
            await self.send_error(interaction, f'Lỗi khi gửi bảng: {e}', ephemeral=True)

    @app_commands.guild_only()
    @app_commands.default_permissions(administrator=True)
    @ticket_group.command(name='panel', description='Gửi lại panel ticket theo cấu hình đã lưu')
    async def ticket_panel(self, interaction: discord.Interaction) -> None:
        guild = interaction.guild
        if not guild:
            return
        config = self.get_guild_config(guild.id)
        if not config or not config.get('panel_channel_id'):
            return await self.send_error(interaction, 'Server chưa cấu hình ticket.', ephemeral=True)
        channel = guild.get_channel(config['panel_channel_id'])
        if not isinstance(channel, discord.TextChannel):
            return await self.send_error(interaction, 'Kênh panel không hợp lệ.', ephemeral=True)
        try:
            await self.render_ticket_panel(channel, guild)
        except discord.Forbidden:
            return await self.send_error(interaction, 'Bot thiếu quyền gửi tin nhắn ở kênh panel.', ephemeral=True)
        await self.send_success(interaction, '✅ Đã gửi lại panel ticket.', ephemeral=True)

    @app_commands.guild_only()
    @ticket_group.command(name='list', description='Xem danh sách ticket đang mở')
    async def ticket_list(self, interaction: discord.Interaction) -> None:
        guild = interaction.guild
        if not guild or not isinstance(interaction.user, discord.Member):
            return
        config = self.get_guild_config(guild.id)
        if not await self.is_staff_or_admin(interaction.user, config):
            return await self.send_error(interaction, 'Bạn không có quyền xem danh sách ticket.', ephemeral=True)
        lines = []
        for ticket in self.ticket_data.get('tickets', {}).values():
            if ticket.get('guild_id') == guild.id and ticket.get('status') in ('open', 'claimed'):
                lines.append(self.format_ticket_record(ticket))
        if not lines:
            return await self.send_success(interaction, 'Hiện không có ticket mở nào.', ephemeral=True)
        embed = discord.Embed(title='📋 Danh sách ticket đang mở', description='\n'.join(lines), color=discord.Color.blue(), timestamp=datetime.datetime.utcnow())
        await self.response_with_embed(interaction, embed, ephemeral=True)

    @app_commands.guild_only()
    @ticket_group.command(name='info', description='Xem thông tin ticket hiện tại hoặc theo channel')
    @app_commands.describe(channel='Kênh ticket cần xem thông tin')
    async def ticket_info(self, interaction: discord.Interaction, channel: Optional[discord.TextChannel] = None) -> None:
        guild = interaction.guild
        member = interaction.user
        if not guild or not isinstance(member, discord.Member):
            return
        target_channel = channel or interaction.channel
        if not isinstance(target_channel, discord.TextChannel):
            return await self.send_error(interaction, 'Vui lòng chỉ định một kênh text hợp lệ.', ephemeral=True)
        ticket = self.get_ticket(target_channel.id)
        if not ticket:
            return await self.send_error(interaction, 'Kênh này không phải ticket hoặc chưa có dữ liệu.', ephemeral=True)
        if ticket.get('owner_id') != member.id and not await self.is_staff_or_admin(member, self.get_guild_config(guild.id)):
            return await self.send_error(interaction, 'Bạn không có quyền xem thông tin ticket này.', ephemeral=True)
        embed = discord.Embed(title='ℹ️ Thông tin ticket', color=discord.Color.blurple(), timestamp=datetime.datetime.utcnow())
        embed.add_field(name='Ticket', value=f'`#{ticket.get("ticket_number"):03d}`', inline=True)
        embed.add_field(name='Trạng thái', value=ticket.get('status'), inline=True)
        embed.add_field(name='Chủ ticket', value=f'<@{ticket.get("owner_id")}>', inline=True)
        if ticket.get('claimed_by'):
            embed.add_field(name='Claimed by', value=f'<@{ticket.get("claimed_by")}>', inline=True)
        embed.add_field(name='Kênh', value=f'<#{ticket.get("channel_id")}>', inline=False)
        if ticket.get('closed_at'):
            embed.add_field(name='Đóng lúc', value=ticket.get('closed_at'), inline=True)
        if ticket.get('close_reason'):
            embed.add_field(name='Lý do đóng', value=ticket.get('close_reason'), inline=False)
        embed.set_footer(text='Ticket system')
        await self.response_with_embed(interaction, embed, ephemeral=True)

    @app_commands.guild_only()
    @ticket_group.command(name='close', description='Đóng ticket bằng command')
    async def ticket_close(self, interaction: discord.Interaction, reason: Optional[str] = None) -> None:
        if not interaction.channel or not isinstance(interaction.channel, discord.TextChannel):
            return await self.send_error(interaction, 'Lệnh chỉ dùng trong kênh ticket.', ephemeral=True)
        member = interaction.user
        if not isinstance(member, discord.Member):
            return
        await self.close_ticket(interaction, interaction.channel.id, reason or 'Không có lý do')

    @app_commands.guild_only()
    @ticket_group.command(name='reopen', description='Mở lại ticket bằng command')
    async def ticket_reopen(self, interaction: discord.Interaction) -> None:
        if not interaction.channel or not isinstance(interaction.channel, discord.TextChannel):
            return await self.send_error(interaction, 'Lệnh chỉ dùng trong kênh ticket.', ephemeral=True)
        await self.reopen_ticket(interaction, interaction.channel.id)

    @app_commands.guild_only()
    @ticket_group.command(name='add', description='Thêm user vào ticket')
    async def ticket_add(self, interaction: discord.Interaction, member: discord.Member) -> None:
        if not interaction.channel or not isinstance(interaction.channel, discord.TextChannel):
            return await self.send_error(interaction, 'Lệnh chỉ dùng trong kênh ticket.', ephemeral=True)
        await self.modify_ticket_user(interaction, interaction.channel.id, str(member.id), add=True)

    @app_commands.guild_only()
    @ticket_group.command(name='remove', description='Xóa user khỏi ticket')
    async def ticket_remove(self, interaction: discord.Interaction, member: discord.Member) -> None:
        if not interaction.channel or not isinstance(interaction.channel, discord.TextChannel):
            return await self.send_error(interaction, 'Lệnh chỉ dùng trong kênh ticket.', ephemeral=True)
        await self.modify_ticket_user(interaction, interaction.channel.id, str(member.id), add=False)

    @app_commands.guild_only()
    @ticket_group.command(name='rename', description='Đổi tên ticket')
    async def ticket_rename(self, interaction: discord.Interaction, name: str) -> None:
        if not interaction.channel or not isinstance(interaction.channel, discord.TextChannel):
            return await self.send_error(interaction, 'Lệnh chỉ dùng trong kênh ticket.', ephemeral=True)
        await self.rename_ticket(interaction, interaction.channel.id, name)

    @app_commands.guild_only()
    @ticket_group.command(name='transcript', description='Tạo transcript ticket')
    async def ticket_transcript(self, interaction: discord.Interaction) -> None:
        if not interaction.channel or not isinstance(interaction.channel, discord.TextChannel):
            return await self.send_error(interaction, 'Lệnh chỉ dùng trong kênh ticket.', ephemeral=True)
        ticket = self.get_ticket(interaction.channel.id)
        if not ticket:
            return await self.send_error(interaction, 'Đây không phải là kênh ticket.', ephemeral=True)
        await interaction.response.defer(ephemeral=True)
        transcript_file = await self.generate_transcript(interaction.channel, ticket)
        await self.send_transcript_to_log(interaction.guild, interaction.channel, transcript_file, interaction.user)
        await self.send_success(interaction, '✅ Transcript đã được tạo và gửi vào kênh log.', ephemeral=True)

    @app_commands.guild_only()
    @app_commands.default_permissions(administrator=True)
    @ticket_group.command(name='config', description='Xem cấu hình ticket hiện tại')
    async def ticket_config(self, interaction: discord.Interaction) -> None:
        guild = interaction.guild
        if not guild:
            return
        config = self.get_guild_config(guild.id)
        if not config:
            return await self.send_error(interaction, 'Server chưa cấu hình ticket.', ephemeral=True)
        embed = discord.Embed(title='⚙️ Cấu hình ticket', color=discord.Color.blue(), timestamp=datetime.datetime.utcnow())
        embed.add_field(name='Kênh panel', value=f'<#{config.get("panel_channel_id")}>', inline=False)
        embed.add_field(name='Category', value=f'<#{config.get("ticket_category_id")}>', inline=False)
        embed.add_field(name='Role staff', value=f'<@&{config.get("staff_role_id")}>', inline=False)
        embed.add_field(name='Kênh log', value=f'<#{config.get("log_channel_id")}>', inline=False)
        embed.add_field(name='Số ticket đã tạo', value=str(config.get('ticket_count', 0)), inline=True)
        embed.add_field(name='Cập nhật', value=config.get('updated_at', 'N/A'), inline=True)
        await self.response_with_embed(interaction, embed, ephemeral=True)

    @app_commands.guild_only()
    @app_commands.default_permissions(administrator=True)
    @ticket_group.command(name='setting', description='Bảng điều khiển cài đặt Bot')
    async def ticket_setting(self, interaction: discord.Interaction) -> None:
        guild = interaction.guild
        if not guild or not isinstance(interaction.user, discord.Member):
            return
        await interaction.response.defer(ephemeral=True)
        embed = discord.Embed(title='⚙️ Bảng Điều Khiển Bot', description='Bấm vào nút để Bật (Xanh) hoặc Tắt (Đỏ).', color=discord.Color.blue())
        embed.add_field(name='🚫 Chống Spam & Nội Dung', value=(
            '**Chặn Link**: Xoá tin nhắn chứa link.\n'
            '**Chặn Media**: Xoá Ảnh và Video.\n'
            '**Chặn File**: Xoá các file khác.\n'
            '**Chặn Chat**: Chỉ xoá tin nhắn Text (không link/file/sticker).\n'
            '**Spam Nhanh**: Cảnh báo khi chat quá nhanh (2 msg/1s).\n'
            '**Spam Lặp**: Cảnh báo khi chat lặp lại (2 msg/60s).'
        ), inline=False)
        embed.add_field(name='⭐ Tiện Ích', value=(
            '**TikTok**: Tự động tải video không logo.\n'
            '**Leveling**: Hệ thống XP và Role.\n'
            '**Sticky**: Ghim tin nhắn dưới cùng đoạn chat.'
        ), inline=False)
        try:
            from cogs.dashboard import DashboardView
            view = DashboardView(guild.id, interaction.channel.id if interaction.channel else 0)
            await interaction.followup.send(embed=embed, view=view, ephemeral=True)
        except Exception as e:
            await self.send_error(interaction, f'Không thể mở bảng cài đặt: {e}', ephemeral=True)

    @app_commands.guild_only()
    @app_commands.default_permissions(administrator=True)
    @ticket_group.command(name='reset', description='Reset cấu hình ticket của server')
    async def ticket_reset(self, interaction: discord.Interaction) -> None:
        guild = interaction.guild
        if not guild:
            return
        guild_key = str(guild.id)
        if guild_key in self.config_data.get('guilds', {}):
            self.config_data['guilds'][guild_key] = {
                'panel_channel_id': 0,
                'panel_message_id': 0,
                'ticket_category_id': 0,
                'staff_role_id': 0,
                'log_channel_id': 0,
                'ticket_count': 0,
                'created_at': format_timestamp(),
                'updated_at': format_timestamp(),
            }
            await self.write_config()
        await self.send_success(interaction, '✅ Đã reset cấu hình ticket của server.', ephemeral=True)


async def setup(bot: commands.Bot) -> None:
    await bot.add_cog(TicketCog(bot))
