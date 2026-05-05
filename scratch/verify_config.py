import sys
import os
import logging

# Add project root to path
sys.path.append(os.getcwd())

from aetox.core.config_loader import ConfigLoader, AgentConfig, ModelOptions

# Configure logging to see what's happening
logging.basicConfig(level=logging.DEBUG)

def test_config_loader():
    print("--- Testing ConfigLoader ---")
    loader = ConfigLoader()
    
    # Test 1: Global Options
    global_opts = loader._config.global_options
    print(f"Global Temperature: {global_opts.temperature}")
    assert global_opts.temperature == 0.1
    
    # Test 2: Role-specific override (Coder)
    coder_model = loader.get_model("coder")
    coder_opts = loader.get_options("coder")
    print(f"Coder Model: {coder_model}")
    print(f"Coder Options: {coder_opts}")
    
    assert coder_model == "qwen3:8b"
    assert coder_opts["temperature"] == 0.0
    assert coder_opts["num_ctx"] == 32768
    assert coder_opts["top_p"] == 0.9 # Should come from global
    
    # Test 3: Standard role (Planner)
    planner_model = loader.get_model("planner")
    planner_opts = loader.get_options("planner")
    print(f"Planner Model: {planner_model}")
    print(f"Planner Options: {planner_opts}")
    
    assert planner_model == "qwen3:8b"
    assert planner_opts["temperature"] == 0.1
    assert planner_opts["num_ctx"] == 16384
    
    print("\n✅ Verification Successful!")

if __name__ == "__main__":
    test_config_loader()
