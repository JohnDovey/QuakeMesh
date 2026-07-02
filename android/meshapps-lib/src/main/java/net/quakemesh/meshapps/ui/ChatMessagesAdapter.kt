package net.quakemesh.meshapps.ui

import android.view.LayoutInflater
import android.view.View
import android.view.ViewGroup
import android.widget.TextView
import androidx.recyclerview.widget.RecyclerView
import net.quakemesh.meshapps.R

data class ChatLine(
    val senderLabel: String,
    val text: String,
    val outgoing: Boolean,
)

class ChatMessagesAdapter : RecyclerView.Adapter<ChatMessagesAdapter.VH>() {
    private val items = mutableListOf<ChatLine>()

    fun submit(lines: List<ChatLine>) {
        items.clear()
        items.addAll(lines)
        notifyDataSetChanged()
    }

    fun append(line: ChatLine) {
        items.add(line)
        notifyItemInserted(items.size - 1)
    }

    override fun onCreateViewHolder(parent: ViewGroup, viewType: Int): VH {
        val v = LayoutInflater.from(parent.context).inflate(R.layout.item_chat_message, parent, false)
        return VH(v)
    }

    override fun onBindViewHolder(holder: VH, position: Int) = holder.bind(items[position])

    override fun getItemCount(): Int = items.size

    class VH(itemView: View) : RecyclerView.ViewHolder(itemView) {
        private val sender = itemView.findViewById<TextView>(R.id.chat_sender)
        private val body = itemView.findViewById<TextView>(R.id.chat_body)

        fun bind(line: ChatLine) {
            sender.text = if (line.outgoing) "You" else line.senderLabel
            body.text = line.text
        }
    }
}
