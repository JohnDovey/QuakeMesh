package net.quakemesh.meshapps.ui

import android.view.LayoutInflater
import android.view.View
import android.view.ViewGroup
import android.widget.TextView
import androidx.recyclerview.widget.RecyclerView
import net.quakemesh.meshapps.R

data class DiscussLine(
    val authorLabel: String,
    val topic: String,
    val text: String,
)

class DiscussPostsAdapter : RecyclerView.Adapter<DiscussPostsAdapter.VH>() {
    private val items = mutableListOf<DiscussLine>()

    fun submit(lines: List<DiscussLine>) {
        items.clear()
        items.addAll(lines)
        notifyDataSetChanged()
    }

    fun append(line: DiscussLine) {
        items.add(line)
        notifyItemInserted(items.size - 1)
    }

    override fun onCreateViewHolder(parent: ViewGroup, viewType: Int): VH {
        val v = LayoutInflater.from(parent.context).inflate(R.layout.item_discuss_post, parent, false)
        return VH(v)
    }

    override fun onBindViewHolder(holder: VH, position: Int) = holder.bind(items[position])

    override fun getItemCount(): Int = items.size

    class VH(itemView: View) : RecyclerView.ViewHolder(itemView) {
        private val meta = itemView.findViewById<TextView>(R.id.post_meta)
        private val body = itemView.findViewById<TextView>(R.id.post_body)

        fun bind(line: DiscussLine) {
            meta.text = "${line.authorLabel} · ${line.topic}"
            body.text = line.text
        }
    }
}
