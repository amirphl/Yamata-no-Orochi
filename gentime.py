import random
from datetime import datetime, timedelta

# Function to generate random datetimes for each day within a specified time range (08:00:00 to 22:00:00)
def generate_random_datetimes(start_datetime, num_datetimes=50, num_times_per_day=7, start_hour=8, end_hour=22):
    datetimes = []
    current_datetime = start_datetime
    datetimes_per_day = num_datetimes // (num_times_per_day)

    for _ in range(datetimes_per_day):
        daily_datetimes = []
        
        for _ in range(num_times_per_day):
            # Random hour, minute, and second within the range 08:00:00 to 22:00:00
            hour = random.randint(start_hour, end_hour - 1)  # Hour between 08 and 21
            minute = random.randint(0, 59)
            second = random.randint(0, 59)
            
            # Construct the datetime with the random time within the current day
            random_datetime = current_datetime.replace(hour=hour, minute=minute, second=second, microsecond=0)
            daily_datetimes.append(random_datetime)
        
        # Sort the daily datetimes in ascending order
        daily_datetimes.sort()
        
        # Append the sorted datetimes to the final list
        datetimes.extend(daily_datetimes)
        
        # Move to the next day
        current_datetime += timedelta(days=1)
    
    return datetimes[:num_datetimes]

# Function to format the datetime in "YYYY-MM-DD HH:MM:SS" format
def format_datetime(dt):
    return dt.strftime('%Y-%m-%d %H:%M:%S')

# Example usage:
if __name__ == "__main__":
    # Input datetime to start generating from (example: "2025-06-14 11:13:02")
    start_str = "2025-07-10 08:00:00"
    start_datetime = datetime.strptime(start_str, '%Y-%m-%d %H:%M:%S')
    
    # Generate 50 random datetimes
    random_datetimes = generate_random_datetimes(start_datetime, num_datetimes=50, num_times_per_day=7)
    
    # Print the generated datetimes
    for dt in random_datetimes:
        print(format_datetime(dt))

