import psutil
import platform
import datetime
import time
from typing import Dict, Any

class SystemMonitorTool:
    """
    Monitor system resources like CPU, RAM, Disk, and processes.
    """
    def get_stats(self) -> str:
        """Returns a detailed snapshot of system resource usage."""
        # CPU
        cpu_usage = psutil.cpu_percent(interval=0.5)
        cpu_freq = psutil.cpu_freq()
        
        # Memory
        memory = psutil.virtual_memory()
        swap = psutil.swap_memory()
        
        # Disk
        disk = psutil.disk_usage('/')
        disk_io = psutil.disk_io_counters()
        
        # Network
        net_io = psutil.net_io_counters()
        
        stats = (
            f"🖥️ **System Intelligence Report**\n"
            f"**[CPU]** Usage: {cpu_usage}% | Freq: {round(cpu_freq.current/1000, 2)}GHz\n"
            f"**[RAM]** Usage: {memory.percent}% ({round(memory.used/1024**3, 2)}GB / {round(memory.total/1024**3, 2)}GB)\n"
            f"**[Swap]** Usage: {swap.percent}%\n"
            f"**[Disk]** Usage: {disk.percent}% | Free: {round(disk.free/1024**3, 2)}GB\n"
            f"**[Disk I/O]** Read: {round(disk_io.read_bytes/1024**2, 1)}MB | Write: {round(disk_io.write_bytes/1024**2, 1)}MB\n"
            f"**[Network]** Sent: {round(net_io.bytes_sent/1024**2, 1)}MB | Recv: {round(net_io.bytes_recv/1024**2, 1)}MB\n"
        )
        return stats

    def get_top_processes(self, limit: int = 5) -> str:
        """Returns the top N processes by CPU and Memory usage."""
        processes = []
        for proc in psutil.process_iter(['pid', 'name', 'cpu_percent', 'memory_percent']):
            try:
                processes.append(proc.info)
            except (psutil.NoSuchProcess, psutil.AccessDenied):
                pass
        
        # Sort by CPU first, then Memory
        top_cpu = sorted(processes, key=lambda x: x['cpu_percent'], reverse=True)[:limit]
        top_mem = sorted(processes, key=lambda x: x['memory_percent'], reverse=True)[:limit]
        
        res = f"🔥 **System Load Analysis (Top {limit})**\n"
        res += "**By CPU Usage:**\n"
        for p in top_cpu:
            res += f"- {p['name']} (PID: {p['pid']}): {p['cpu_percent']}%\n"
        
        res += "\n**By Memory Usage:**\n"
        for p in top_mem:
            res += f"- {p['name']} (PID: {p['pid']}): {round(p['memory_percent'], 2)}%\n"
        return res

    def get_os_info(self) -> str:
        """Returns accurate OS information, specifically handling Windows 11 and Uptime."""
        os_name = platform.system()
        os_release = platform.release()
        os_version = platform.version()
        
        if os_name == "Windows" and os_release == "10":
            try:
                build = int(os_version.split(".")[-1])
                if build >= 22000:
                    os_release = "11"
            except: pass

        uptime_seconds = time.time() - psutil.boot_time()
        uptime_hours = int(uptime_seconds // 3600)
        uptime_minutes = int((uptime_seconds % 3600) // 60)
        
        info = (
            f"ℹ️ **OS Information**\n"
            f"- OS: {os_name} {os_release} (Build {os_version})\n"
            f"- Node: {platform.node()}\n"
            f"- Arch: {platform.machine()}\n"
            f"- Uptime: {uptime_hours}h {uptime_minutes}m\n"
            f"- Boot Time: {datetime.datetime.fromtimestamp(psutil.boot_time()).strftime('%Y-%m-%d %H:%M:%S')}\n"
        )
        return info
