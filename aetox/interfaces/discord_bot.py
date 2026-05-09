import os
import discord
import logging
import asyncio
import time
from typing import Dict, Any, Tuple, Optional
from discord.ext import commands
from dotenv import load_dotenv

# Load environment variables
load_dotenv()
TOKEN = os.getenv("DISCORD_TOKEN")
ALLOWED_USERS = os.getenv("ALLOWED_USER_IDS", "").split(",")

# Setup logging
logging.basicConfig(level=logging.INFO)
logger = logging.getLogger("aetox.interfaces.discord")

from aetox.core.ollama_client import OllamaClient
from aetox.core.prompt_engine import PromptEngine
from aetox.planner import AetoxPlanner
from aetox.core.dispatcher import Dispatcher
from aetox.memory.working import SessionContext

# Initialize Discord Bot
intents = discord.Intents.default()
intents.message_content = True
bot = commands.Bot(command_prefix="!", intents=intents)

# Multi-User Session Manager
class SessionManager:
    """
    Manages isolated SessionContext and Dispatcher instances for multiple users.
    Includes cleanup logic to prevent memory leaks.
    """
    def __init__(self, max_users: int = 50, ttl: int = 3600):
        self.sessions: Dict[int, Dict[str, Any]] = {}
        self.max_users = max_users
        self.ttl = ttl
        self.lock = asyncio.Lock()

    async def get_resources(self, user_id: int) -> Tuple[SessionContext, Dispatcher]:
        async with self.lock:
            current_time = time.time()
            
            # 1. Cleanup expired sessions
            await self._cleanup(current_time)

            # 2. Return existing or create new
            if user_id in self.sessions:
                self.sessions[user_id]["last_active"] = current_time
                return self.sessions[user_id]

            # 3. Handle capacity
            if len(self.sessions) >= self.max_users:
                oldest_user = min(self.sessions, key=lambda u: self.sessions[u]["last_active"])
                del self.sessions[oldest_user]
                logger.info(f"Session Manager: Evicted oldest session (User: {oldest_user})")

            # 4. Create new instance
            session = SessionContext(chat_history_limit=5)
            dispatcher = Dispatcher()
            
            self.sessions[user_id] = {
                "session": session,
                "dispatcher": dispatcher,
                "last_active": current_time,
                "is_active": True,         # Active by default as requested
                "active_task": None
            }
            logger.info(f"Session Manager: Created new session for User: {user_id}")
            return self.sessions[user_id]

    async def _cleanup(self, current_time: float):
        expired = [u for u, data in self.sessions.items() if (current_time - data["last_active"]) > self.ttl]
        for u in expired:
            # Cancel any running task before cleaning up
            if self.sessions[u].get("active_task"):
                self.sessions[u]["active_task"].cancel()
            del self.sessions[u]
            logger.info(f"Session Manager: Cleaned up expired session for User: {u}")

    def set_active_task(self, user_id: int, task: Optional[asyncio.Task]):
        if user_id in self.sessions:
            self.sessions[user_id]["active_task"] = task

    def get_session_data(self, user_id: int) -> Optional[Dict[str, Any]]:
        return self.sessions.get(user_id)

session_manager = SessionManager(max_users=50, ttl=3600)

class DiscordInterface:
    """
    Asynchronous Interface Layer for AetoxClaw.
    Pipe mode: Discord for output, Terminal for debug/analysis.
    """
    def __init__(self, context: commands.Context):
        self.context = context

    async def stream_chat(self, stream_generator):
        """Streams AI response tokens asynchronously to Discord."""
        message = None
        full_content = ""
        last_update = 0
        update_interval = 0.5

        try:
            async for token in stream_generator:
                if token == "__NOT_CHAT__":
                    return False
                
                full_content += token
                current_time = time.time()
                
                if (current_time - last_update) > update_interval or not message:
                    if not message:
                        message = await self.context.send(full_content + " ▌")
                    else:
                        await message.edit(content=full_content + " ▌")
                    last_update = current_time
            
            if message:
                await message.edit(content=full_content)
            elif full_content:
                await self.context.send(full_content)
            return full_content
        except Exception as e:
            print(f"\n[ERROR] Streaming failed: {e}")
            await self.context.send(f"❌ **ขออภัย ระบบขัดข้อง:** {str(e)}")
            return ""

    async def send_progress(self, message: str):
        """Log progress to Terminal and Discord for visibility."""
        print(f"[PROGRESS] {message}")
        try:
            await self.context.send(f"⏳ {message}")
        except:
            pass

    async def request_approval(self, action: str, details: str) -> bool:
        """Asynchronous permission request."""
        msg = await self.context.send(
            f"⚠️ **ความปลอดภัย:** ต้องการ `{action}`\n"
            f"**รายละเอียด:** `{details}`\n"
            f"กด ✅ เพื่ออนุมัติ หรือ ❌ เพื่อปฏิเสธ"
        )
        await msg.add_reaction("✅")
        await msg.add_reaction("❌")

        def check(reaction, user):
            return user == self.context.author and str(reaction.emoji) in ["✅", "❌"] and reaction.message.id == msg.id

        try:
            reaction, _ = await bot.wait_for("reaction_add", timeout=120.0, check=check)
            return str(reaction.emoji) == "✅"
        except asyncio.TimeoutError:
            await self.context.send("⏳ **หมดเวลา:** ปฏิเสธการดำเนินการ")
            return False

@bot.event
async def on_message(message):
    if message.author == bot.user: return
    
    # Check if message is a command
    if message.content.startswith('!'):
        await bot.process_commands(message)
        return

    # Check if user is allowed
    # Get session data
    session_data = await session_manager.get_resources(message.author.id)
    
    # Check if user is allowed
    if str(message.author.id) not in ALLOWED_USERS and "*" not in ALLOWED_USERS: return

    ctx = await bot.get_context(message)
    
    # Cancel previous task if still running
    if session_data.get("active_task") and not session_data["active_task"].done():
        session_data["active_task"].cancel()
        await ctx.send("🔄 **เริ่มงานใหม่และยกเลิกงานเดิม...**")

    # Start new task
    task = asyncio.create_task(handle_task_pipe(ctx, message.content.strip()))
    session_manager.set_active_task(message.author.id, task)

# --- BOT COMMANDS ---

@bot.command(name="start")
async def start_cmd(ctx):
    """Activate the system and check health."""
    session_data = await session_manager.get_resources(ctx.author.id)
    session_data["is_active"] = True
    
    ollama = OllamaClient()
    is_ok = await ollama.check_health()
    
    status_emoji = "✅" if is_ok else "❌"
    health_text = "Ollama พร้อมทำงาน" if is_ok else "Ollama ไม่ได้เปิดอยู่ (โปรดเปิดโปรแกรม Ollama)"
    
    await ctx.send(
        f"🌌 **AetoxClaw Online**\n"
        f"สถานะ: `เปิดการทำงาน (Active)`\n"
        f"ระบบ AI: {status_emoji} {health_text}\n"
        f"ตอนนี้คุณสามารถพิมพ์สั่งงานได้โดยตรงเลยครับ!"
    )

@bot.command(name="stop")
async def stop_cmd(ctx):
    """Deactivate the system and cancel any running task."""
    session_data = await session_manager.get_resources(ctx.author.id)
    session_data["is_active"] = False
    
    if session_data.get("active_task") and not session_data["active_task"].done():
        session_data["active_task"].cancel()
        await ctx.send("🛑 **หยุดการทำงานและยกเลิกงานที่ค้างอยู่เรียบร้อยครับ**")
    else:
        await ctx.send("💤 **เข้าสู่โหมด Standby เรียบร้อยครับ**")

@bot.command(name="status")
async def status_cmd(ctx):
    """Check system status."""
    session_data = await session_manager.get_resources(ctx.author.id)
    is_active = session_data.get("is_active")
    
    ollama = OllamaClient()
    is_ok = await ollama.check_health()
    
    msg = (
        f"📊 **ระบบ AetoxClaw**\n"
        f"โหมดปัจจุบัน: `{'Active' if is_active else 'Standby'}`\n"
        f"Ollama Connectivity: `{'Online' if is_ok else 'Offline'}`\n"
    )
    if not is_ok:
        msg += "⚠️ *คำเตือน: โปรดตรวจสอบว่า Ollama รันอยู่ที่ localhost:11434*"
    
    await ctx.send(msg)

@bot.command(name="shutdown")
async def shutdown_cmd(ctx):
    """Shutdown the bot (Admin only)."""
    if str(ctx.author.id) not in ALLOWED_USERS:
        await ctx.send("❌ คุณไม่มีสิทธิ์ในการสั่งปิดระบบ")
        return
    
    await ctx.send("👋 **กำลังปิดระบบ AetoxClaw... ลาก่อนครับ**")
    await bot.close()

async def handle_task_pipe(ctx, goal):
    """Main entry point — Unified Streaming Pipeline."""
    if not goal: return
    
    # 🧠 Get resources
    session_data = await session_manager.get_resources(ctx.author.id)
    session = session_data["session"]
    dispatcher = session_data["dispatcher"]
    
    interface = DiscordInterface(ctx)
    dispatcher.progress_callback = interface.send_progress
    dispatcher.executor.permission_manager.approval_callback = interface.request_approval

    try:
        async with ctx.typing():
            # เช็ค Ollama ก่อนเริ่มงาน
            if not await dispatcher.executor.client.check_health():
                await ctx.send("❌ **ข้อผิดพลาด:** ไม่สามารถเชื่อมต่อกับ Ollama ได้ โปรดเปิดโปรแกรม Ollama ก่อนครับ")
                return

            # ดึงประวัติจาก SessionContext
            history_str = session.get_history_as_string()
            minimal_context = {"history": history_str}
            
            extraction = await dispatcher.executor.extract_action({"description": goal}, minimal_context)
            
            analysis = extraction.get("analysis", "วิเคราะห์งาน...")
            print(f"\n[GOAL] {goal}\n[ANALYSIS] {analysis}\n")

            if extraction.get("estimated_steps", 1) > 2:
                # --- PLANNING LANE ---
                await ctx.send(f"🤖 **วิเคราะห์:** {analysis}")
                status_msg = await ctx.send("⏳ **กำลังเตรียมแผนงาน...**")
                planner = AetoxPlanner()
                plan = await planner.create_plan(goal)
                await status_msg.delete()
                
                steps_msg = "📝 **แผนการทำงาน:**\n" + "\n".join([f"- {s.get('description')}" for s in plan.get('steps', [])])
                await ctx.send(steps_msg)
                
                if await interface.request_approval("ดำเนินการตามแผน", goal):
                    result = await dispatcher.run_plan(plan)
                    await ctx.send("🏁 **ภารกิจเสร็จสิ้น!**")
                    # บันทึกผลลัพธ์ของ Plan ลง session history
                    session.add_exchange(goal, f"Plan completed: {result.get('status')}")
            else:
                # --- UNIFIED STREAMING PIPE ---
                result = await dispatcher.executor.run_action(extraction, minimal_context)
                
                async def unified_generator():
                    if result.get("status") == "success":
                        output = result.get("output", "")
                        if len(output) > 600:
                            yield "📝 **[สรุปเนื้อหาสำคัญ]:**\n\n"
                            sum_prompt = f"สรุปเนื้อหาต่อไปนี้ให้สั้นและเป็นประเด็นสำคัญในภาษาไทย:\n\n{output[:8000]}"
                            async for token in dispatcher.executor.run_chat_stream(sum_prompt):
                                yield token
                        else:
                            yield output or "สำเร็จเรียบร้อยครับ"
                    elif result.get("status") == "chat":
                        async for token in dispatcher.executor.run_chat_stream(goal, context=result.get("output")):
                            yield token
                    else:
                        yield f"❌ **ผิดพลาด:** {result.get('error', 'Unknown Error')}"

                final_response = await interface.stream_chat(unified_generator())
                if final_response:
                    # บันทึกลง SessionContext
                    session.add_exchange(goal, final_response if isinstance(final_response, str) else str(final_response))
    except asyncio.CancelledError:
        logger.info(f"Task for user {ctx.author.id} was cancelled.")
        # No need to send message here as !stop already sends one
    except Exception as e:
        logger.error(f"Error in handle_task_pipe: {e}")
        await ctx.send(f"❌ **เกิดข้อผิดพลาด:** {str(e)}")

if __name__ == "__main__":
    if TOKEN: bot.run(TOKEN)
    else: logger.error("No TOKEN found.")
