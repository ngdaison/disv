
# Global Cache for Deduplication
PROCESSED_MESSAGES = set()

def should_process(message_id):
    if message_id in PROCESSED_MESSAGES:
        return False
    
    PROCESSED_MESSAGES.add(message_id)
    
    # Cleanup if too big
    if len(PROCESSED_MESSAGES) > 1000:
        PROCESSED_MESSAGES.clear()
        
    return True
