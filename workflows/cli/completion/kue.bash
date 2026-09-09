# bash completion for kue                                   -*- shell-script -*-
#
# Install:
#   source <(kue completion bash)
# or drop this file into your bash-completion directory, e.g.
#   kue completion bash > /etc/bash_completion.d/kue

_kue() {
    local cur prev cword words
    if declare -F _init_completion >/dev/null 2>&1; then
        _init_completion || return
    else
        COMPREPLY=()
        cur="${COMP_WORDS[COMP_CWORD]}"
        prev="${COMP_WORDS[COMP_CWORD-1]}"
        words=("${COMP_WORDS[@]}")
        cword=$COMP_CWORD
    fi

    local commands="run modules transitions create add update show inspect \
wsl get upload list delete status \
register login logout whoami profile install package completion \
templates help"

    local global_opts="-v --verbose --debug -q --quiet -h --help \
--modules --workflows -H --host -c --config"

    # locate the top-level command (first non-flag word after "kue")
    local cmd="" sub="" i
    for (( i = 1; i < cword; i++ )); do
        case "${words[i]}" in
            -*) ;;
            *) cmd="${words[i]}"; break ;;
        esac
    done

    if [[ -z $cmd ]]; then
        if [[ $cur == -* ]]; then
            COMPREPLY=( $(compgen -W "$global_opts" -- "$cur") )
        else
            COMPREPLY=( $(compgen -W "$commands" -- "$cur") )
        fi
        return
    fi

    # locate the subcommand (next non-flag word)
    for (( i = i + 1; i < cword; i++ )); do
        case "${words[i]}" in
            -*) ;;
            *) sub="${words[i]}"; break ;;
        esac
    done

    if [[ -z $sub ]]; then
        case "$cmd" in
            add)
                COMPREPLY=( $(compgen -W "module workflow feature solution package transition" -- "$cur") ); return ;;
            wsl)
                COMPREPLY=( $(compgen -W "get inspect status upload list delete" -- "$cur") ); return ;;
            package)
                COMPREPLY=( $(compgen -W "add update search install list list-local enable publish show" -- "$cur") ); return ;;
            profile)
                COMPREPLY=( $(compgen -W "get update" -- "$cur") ); return ;;
            templates)
                COMPREPLY=( $(compgen -W "download update clear status" -- "$cur") ); return ;;
            completion)
                COMPREPLY=( $(compgen -W "bash zsh" -- "$cur") ); return ;;
            help)
                COMPREPLY=( $(compgen -W "examples env $commands" -- "$cur") ); return ;;
        esac
    fi

    case "$cmd" in
        run|r|install|i|inspect|upload)
            if [[ $cur != -* ]]; then
                local files
                files=$(compgen -f -X '!*.@(wsl|swsl)' -- "$cur")
                COMPREPLY=( $files $(compgen -d -- "$cur") )
                return
            fi
            ;;
    esac

    if [[ $cur == -* ]]; then
        COMPREPLY=( $(compgen -W "$global_opts -j --json -f --force -n --name -o --output -s --search -P --public -M --method" -- "$cur") )
        return
    fi
}

complete -F _kue kue
