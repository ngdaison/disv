import discord
from discord.ext import commands, tasks
from utils.data_handler import load_data, save_data, get_guild_data
import datetime
import logging

logger = logging.getLogger(__name__)

class AutoRoles(commands.Cog):
    def __init__(self, bot):
        self.bot = bot
        self.check_auto_roles.start()

    @commands.Cog.listener()
    async def on_member_join(self, member):
        data = load_data()
        g_data = get_guild_data(data, member.guild.id)
        
        # Store join timestamp
        g_data["join_times"][str(member.id)] = member.joined_at.isoformat()
        save_data(data)
        
        # Assign immediate roles (if any set to 0 hours)
        auto_roles = g_data.get("auto_join_roles", {})
        for role_id_str, hours in auto_roles.items():
            if hours == 0:
                role = member.guild.get_role(int(role_id_str))
                if role and member.guild.me.top_role > role:
                    try:
                        await member.add_roles(role)
                        logger.info("Auto role assigned on join: %s to %s in %s", role.name, member, member.guild.name)
                    except Exception as e:
                        logger.error("Failed to add role %s to %s on join: %s", role_id_str, member, e)

    @commands.Cog.listener()
    async def on_ready(self):
        # Populate join_times for existing members if not already present
        data = load_data()
        for guild in self.bot.guilds:
            g_data = get_guild_data(data, guild.id)
            join_times = g_data.get("join_times", {})
            for member in guild.members:
                if str(member.id) not in join_times:
                    join_times[str(member.id)] = member.joined_at.isoformat()
            g_data["join_times"] = join_times
        save_data(data)
        print("Populated join_times for existing members.")

    @tasks.loop(minutes=1)
    async def check_auto_roles(self):
        data = load_data()
        now = datetime.datetime.now(datetime.timezone.utc)
        
        for guild_id_str, g_data in data.items():
            guild = self.bot.get_guild(int(guild_id_str))
            if not guild:
                continue
            
            auto_roles = g_data.get("auto_join_roles", {})
            join_times = g_data.get("join_times", {})
            
            for user_id_str, join_time_iso in join_times.items():
                try:
                    join_time = datetime.datetime.fromisoformat(join_time_iso.replace('Z', '+00:00'))
                    if join_time.tzinfo is None:
                        join_time = join_time.replace(tzinfo=datetime.timezone.utc)
                except:
                    continue
                
                for role_id_str, hours in auto_roles.items():
                    delay_seconds = hours * 3600
                    if (now - join_time).total_seconds() >= delay_seconds:
                        member = guild.get_member(int(user_id_str))
                        role = guild.get_role(int(role_id_str))
                        
                        if member and role and guild.me.top_role > role:
                            if role not in member.roles:
                                try:
                                    await member.add_roles(role)
                                    logger.info("Auto role assigned by scheduler: %s to %s in %s", role.name, member, guild.name)
                                except Exception as e:
                                    logger.error("Failed to add role %s to %s in scheduled check: %s", role_id_str, member, e)

    @check_auto_roles.before_loop
    async def before_check_auto_roles(self):
        await self.bot.wait_until_ready()

async def setup(bot):
    await bot.add_cog(AutoRoles(bot))