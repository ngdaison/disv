import discord
from discord.ext import commands
import json
import os
import aiohttp
import asyncio
import logging
import sys
import threading
import traceback
import warnings
from typing import Optional

if sys.platform == "win32":
    try:
        with warnings.catch_warnings():
            warnings.filterwarnings("ignore", category=DeprecationWarning)
            asyncio.set_event_loop_policy(asyncio.WindowsSelectorEventLoopPolicy())
    except Exception:
        pass

# Load Config
with open('config.json', 'r', encoding='utf-8') as f:
    config = json.load(f)

intents = discord.Intents.default()
intents.message_content = True
intents.members = True

class BotClient(commands.Bot):
    def __init__(self, *args, **kwargs):
        super().__init__(*args, **kwargs)
        self.http_session: Optional[aiohttp.ClientSession] = None
        self.chatai_bot: Optional[commands.Bot] = None
        self.chatai_task: Optional[asyncio.Task] = None

    async def setup_hook(self) -> None:
        log.info("Starting bot setup_hook")
        self.http_session = aiohttp.ClientSession()
        asyncio.get_running_loop().set_exception_handler(self.async_exception_handler)

        chatai_token = config.get('chatai_token', '')
        if chatai_token:
            self.chatai_bot = commands.Bot(command_prefix=config['bot_prefix'], intents=intents)
            self.chatai_bot.is_chatai = True
            self.chatai_bot.http_session = self.http_session
            self.chatai_bot.main_bot = self
            try:
                await self.chatai_bot.load_extension('cogs.chatai')
            except Exception as e:
                log.exception(f'Failed loading chatai into chatai_bot: {e}')
            self.chatai_task = asyncio.create_task(self.chatai_bot.start(chatai_token))
        else:
            self.chatai_bot = None
            self.chatai_task = None
            try:
                await self.load_extension('cogs.chatai')
            except Exception as e:
                log.exception(f'Failed loading chatai into main bot: {e}')

        for extension in [
            'cogs.tiktok',
            'cogs.leveling',
            'cogs.moderation',
            'cogs.utility',
            'cogs.dashboard',
            'cogs.ticket',
            'cogs.autoroles',
            'cogs.noitu'
        ]:
            try:
                await self.load_extension(extension)
                log.info(f'Loaded extension: {extension}')
            except Exception as e:
                log.exception(f'Failed loading {extension}: {e}')

    def async_exception_handler(self, loop, context):
        log.error("Asyncio exception in loop: %s", context)
        if context.get('exception'):
            log.exception(context['exception'])

    async def close(self) -> None:
        if self.chatai_task is not None:
            self.chatai_task.cancel()
            try:
                await self.chatai_task
            except Exception:
                pass
        if self.chatai_bot is not None:
            try:
                await self.chatai_bot.close()
            except Exception:
                pass
        if self.http_session is not None and not self.http_session.closed:
            await self.http_session.close()
        await super().close()

bot = BotClient(command_prefix=config['bot_prefix'], intents=intents)

# Basic logging to help diagnose why the process may exit
logging.basicConfig(level=logging.INFO)
log = logging.getLogger(__name__)
logging.getLogger('discord').setLevel(logging.INFO)

@bot.event
async def on_connect():
    log.info("Bot connected to gateway")


@bot.event
async def on_ready():
    print(f"Logged in as {bot.user}")
    await bot.change_presence(activity=discord.Game(name=config['status_message']))
    try:
        synced = await bot.tree.sync()
        print(f"Synced {len(synced)} slash commands.")
    except Exception as e:
        print(f"Sync error: {e}")


@bot.event
async def on_resumed():
    log.info("Bot session resumed")


@bot.event
async def on_disconnect():
    log.info("Bot disconnected from gateway")


@bot.event
async def on_error(event, *args, **kwargs):
    log.exception(f"Unhandled exception in event {event}")


def handle_uncaught_exception(exc_type, exc_value, exc_traceback):
    if issubclass(exc_type, KeyboardInterrupt):
        log.info("KeyboardInterrupt received")
        return
    log.error("Uncaught exception", exc_info=(exc_type, exc_value, exc_traceback))

if __name__ == "__main__":
    sys.excepthook = handle_uncaught_exception
    if hasattr(threading, 'excepthook'):
        def thread_exception_handler(args):
            log.error("Uncaught thread exception", exc_info=(args.exc_type, args.exc_value, args.exc_traceback))
        threading.excepthook = thread_exception_handler
    try:
        log.info("Starting bot.run")
        bot.run(config['bot_token'])
        log.info("bot.run returned normally")
    except Exception as e:
        log.exception(f"Bot failed to start: {e}")
    finally:
        log.info("Bot process ended")
