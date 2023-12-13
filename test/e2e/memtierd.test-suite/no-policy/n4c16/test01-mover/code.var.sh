memtierd-setup

# Create swap
zram-install
zram-swap off
zram-swap 4G

next-round() {
    local round_counter_var=$1
    local round_counter_val=${!1}
    local round_counter_max=$2
    local round_delay=$3
    if [[ "$round_counter_val" -ge "$round_counter_max" ]]; then
        return 1
    fi
    eval "$round_counter_var=$(($round_counter_val + 1))"
    sleep $round_delay
    return 0
}

# In general, it takes around 5 seconds to finish moving out
match-moved() {
    local moved_regexp=$1
    local target_numa_node=$2
    local min_seconds=$3
    local max_seconds=$4
    round_number=0
    while ! (
        memtierd-command "stats -t move_pages -f csv | awk -F, \"{print \\\$6}\""
        grep ${moved_regexp} <<<$COMMAND_OUTPUT
    ); do
        echo "grep MOVED value to the target numa node ${target_numa_node} matching ${moved_regexp} not found"
        next-round round_number $max_seconds 1s || {
            error "timeout: memtierd did not expected amount of memory"
        }
    done
    if [[ "$round_number" -lt "$min_seconds" ]]; then
        error "too fast page moves, expected at least $min_seconds seconds, got $round_number"
    fi
    echo "pages moved in less than $((round_number + 1)) seconds that is in expected range [$min_seconds, $max_seconds]"
}

match-pid-pageout() {
    local pid=$1
    local pageout_regexp=$2
    local min_seconds=$3
    local max_seconds=$4
    round_number=0
    while ! (
        memtierd-command "swap -pid $pid -status | awk \"{print \\\$6}\""
        grep ${pageout_regexp} <<<$COMMAND_OUTPUT
    ); do
        echo "grep PAGEOUT value matching ${pageout_regexp} not found"
        next-round round_number $max_seconds 1s || {
            memtierd-command "swap -pid $pid -status"
            error "timeout: memtierd did not swap expected amount of memory"
        }
    done
    if [[ "$round_number" -lt "$min_seconds" ]]; then
        error "too fast swapping, expected at least $min_seconds seconds, got $round_number"
    fi
    echo "swapping out in less than $((round_number + 1)) seconds that is in expected range [$min_seconds, $max_seconds]"
}

# By default, -config {"IntervalMs":10,"Bandwidth":100}
mover_conf=(
    '-config-dump'
    '-config {"IntervalMs":1000,"Bandwidth":100000}'
    '-config {"IntervalMs":10,"Bandwidth":200}'
    '-config {"IntervalMs":20,"Bandwidth":500}'
)

mover_min_max_seconds=(
    "0 10"
    "0 1"
    "2 4"
    "1 2"
)
echo -e "\n=== scenario 1: test moving memories among numa nodes ===\n"

# Test that 700M+ is moved to numa node {0..3} from a process
MEME_BS=720M MEME_BWC=1 MEME_BWS=720M memtierd-meme-start

for target_numa_node in {0..3}; do
    MEMTIERD_YAML=""
    memtierd-start
    memtierd-command "pages -pid ${MEME_PID}"
    memtierd-command "mover ${mover_conf[$target_numa_node]} -pages-to ${target_numa_node}"
    echo "waiting 700M+ to be moved to ${target_numa_node}"
    match-moved "${target_numa_node}\:0\.7[0-9][0-9]" ${target_numa_node} ${mover_min_max_seconds[$target_numa_node]}
    memtierd-stop
done

memtierd-meme-stop

echo -e "\n=== scenario 2: test swapping out memories ===\n"

echo -e "\n=== scenario 2.1: test swapping out memories for one process with various mover configurations ===\n"

for i in {0..3}; do
    # Test that 700M+ is swapped out
    MEME_BS=1G MEME_BWC=1 MEME_BWS=280M memtierd-meme-start
    MEMTIERD_YAML=""
    memtierd-start
    memtierd-command "mover ${mover_conf[$i]}\nswap -out -pids $MEME_PID -mover\nmover -wait"
    match-pid-pageout $MEME_PID "7[0-9][0-9]" ${mover_min_max_seconds[$i]}
    memtierd-meme-stop
    memtierd-stop
done

echo -e "\n=== scenario 2.2: test swapping out memories for multiple processes ===\n"

# Start 3 meme, each should have 700M+ swapped out
for i in {0..2}; do
    MEME_BS=1G MEME_BWC=1 MEME_BWS=280M memtierd-meme-start
    PID_VAR="MEME${i}_PID"
    eval "${PID_VAR}=${MEME_PID}"
done
