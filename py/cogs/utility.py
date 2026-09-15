import discord
from discord.ext import commands
from utils.data_handler import load_data, save_data, get_guild_data, get_channel_settings
from utils.antispam import should_process
import asyncio

class Utility(commands.Cog):
    def __init__(self, bot):
        self.bot = bot

    @commands.Cog.listener()
    async def on_message_delete(self, message):
        if message.author.id == self.bot.user.id:
            data = load_data()
            # We don't have guild info if message isn't cached? 
            # message reference might be partial. 
            if message.guild:
                g_data = data.get(str(message.guild.id))
                if g_data:
                    s_settings = g_data.get("settings", {}).get(str(message.channel.id))
                    if s_settings and s_settings.get('sticky_last_id') == message.id:
                        s_settings['sticky_content'] = None
                        s_settings['sticky_last_id'] = None
                        save_data(data)

    @commands.Cog.listener()
    async def on_message(self, message):
        if message.author.bot:
            return
            
        # Scoped Sticky
        if not should_process(f"sticky_{message.id}"):
            return

        data = load_data()
        c_settings = get_channel_settings(data, message.guild.id, message.channel.id)
        
        sticky_text = c_settings.get("sticky_content")
        if sticky_text:
            # Wait for 3 seconds as requested
            await asyncio.sleep(3)
            
            # RELOAD DATA to prevent race conditions if multiple messages came in during sleep
            data = load_data()
            c_settings = get_channel_settings(data, message.guild.id, message.channel.id)
            # Re-check if sticky is still enabled
            sticky_text = c_settings.get("sticky_content")
            if not sticky_text:
                return

            # 1. Update DB to prevent on_message_delete from disabling the feature
            # because we are about to delete the message ourselves.
            last_id = c_settings.get("sticky_last_id")
            if last_id:
                c_settings['sticky_last_id'] = None 
                save_data(data)
                
                # 2. Optimized Delete (No Fetch)
                try:
                    # Create a PartialMessage to delete without API fetch call
                    partial_msg = message.channel.get_partial_message(last_id)
                    await partial_msg.delete()
                except discord.NotFound:
                    pass # Already deleted
                except discord.Forbidden:
                    pass # Keep going if we can't delete (perm issue)
                except Exception as e:
                    print(f"Sticky disable error: {e}")
            
            # 3. Send New
            try:
                new_msg = await message.channel.send(sticky_text)
                c_settings["sticky_last_id"] = new_msg.id
                save_data(data)
            except Exception as e:
                print(f"Failed to send sticky: {e}")

async def setup(bot):
    await bot.add_cog(Utility(bot))
