import discord
from discord.ext import commands
from utils.data_handler import load_data, save_data, get_guild_data, get_channel_settings
from utils.antispam import should_process

class Leveling(commands.Cog):
    def __init__(self, bot):
        self.bot = bot

    @commands.Cog.listener()
    async def on_message(self, message):
        if message.author.bot:
            return

        # Check enabled
        data = load_data()
        c_settings = get_channel_settings(data, message.guild.id, message.channel.id)
        if not c_settings['level_enabled']:
            return

        # Scoped Deduplication
        if not should_process(f"level_{message.id}"):
            return

        g_data = get_guild_data(data, message.guild.id)
        uid = str(message.author.id)
        
        # Init user
        if uid not in g_data["users"]: 
            g_data["users"][uid] = {"xp": 0, "level": 1, "warnings": []}
        
        u = g_data["users"][uid]
        u["xp"] += 1
        
        # Level Up Logic
        needed = u["level"] * 10
        if u["xp"] >= needed:
            u["level"] += 1
            u["xp"] = 0
            await message.channel.send(f"🎉 {message.author.mention} lên cấp {u['level']}!")
            
            # Role Reward
            lvl_str = str(u["level"])
            if lvl_str in g_data.get("level_roles", {}):
                rid = int(g_data["level_roles"][lvl_str])
                role = message.guild.get_role(rid)
                if role:
                    try: await message.author.add_roles(role)
                    except: pass
        
        save_data(data)

async def setup(bot):
    await bot.add_cog(Leveling(bot))
